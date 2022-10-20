package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type clientProcessorFMP4 struct {
	segmentQueue *clientSegmentQueue
	logger       ClientLogger

	initVideoTrack   *fmp4.InitTrack
	initAudioTrack   *fmp4.InitTrack
	videoProc        *clientProcessorFMP4Track
	audioProc        *clientProcessorFMP4Track
	clockInitialized bool
	startBaseTime    uint64

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) (*clientProcessorFMP4, error) {
	initVideoTrack, initAudioTrack, err := fmp4.InitRead(initFile)
	if err != nil {
		return nil, err
	}

	var videoTrack *gortsplib.TrackH264
	if initVideoTrack != nil {
		videoTrack = initVideoTrack.Track.(*gortsplib.TrackH264)
	}

	var audioTrack *gortsplib.TrackMPEG4Audio
	if initAudioTrack != nil {
		audioTrack = initAudioTrack.Track.(*gortsplib.TrackMPEG4Audio)
	}

	err = onTracks(videoTrack, audioTrack)
	if err != nil {
		return nil, err
	}

	p := &clientProcessorFMP4{
		segmentQueue:     segmentQueue,
		logger:           logger,
		initVideoTrack:   initVideoTrack,
		initAudioTrack:   initAudioTrack,
		subpartProcessed: make(chan struct{}),
	}

	if videoTrack != nil {
		p.videoProc = newClientProcessorFMP4Track(
			initVideoTrack.TimeScale,
			p.onSubpartProcessed,
			func(pts time.Duration, payload []byte) error {
				nalus, err := h264.AVCCUnmarshal(payload)
				if err != nil {
					return err
				}

				onVideoData(pts, nalus)
				return nil
			},
		)
		rp.add(p.videoProc)
	}

	if audioTrack != nil {
		p.audioProc = newClientProcessorFMP4Track(
			initAudioTrack.TimeScale,
			p.onSubpartProcessed,
			func(pts time.Duration, payload []byte) error {
				return nil
			},
		)
		rp.add(p.audioProc)
	}

	return p, nil
}

func (p *clientProcessorFMP4) run(ctx context.Context) error {
	for {
		seg, ok := p.segmentQueue.pull(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := p.processSegment(ctx, seg)
		if err != nil {
			return err
		}
	}
}

func (p *clientProcessorFMP4) processSegment(ctx context.Context, byts []byte) error {
	p.logger.Log(logger.Debug, "processing segment")

	subparts, err := fmp4.PartRead(byts)
	if err != nil {
		return err
	}

	processingCount := 0

	for _, track := range subparts {
		if !p.clockInitialized {
			p.clockInitialized = true
			p.startBaseTime = track.BaseTime

			now := time.Now()
			if p.videoProc != nil {
				p.videoProc.startRTC = now
			}
			if p.audioProc != nil {
				p.audioProc.startRTC = now
			}
		}

		track.BaseTime -= p.startBaseTime

		if p.initVideoTrack != nil && track.ID == p.initVideoTrack.ID {
			select {
			case p.videoProc.queue <- track:
			case <-ctx.Done():
				return fmt.Errorf("terminated")
			}

			processingCount++
		}
	}

	for i := 0; i < processingCount; i++ {
		select {
		case <-p.subpartProcessed:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

func (p *clientProcessorFMP4) onSubpartProcessed(ctx context.Context) {
	select {
	case p.subpartProcessed <- struct{}{}:
	case <-ctx.Done():
	}
}
