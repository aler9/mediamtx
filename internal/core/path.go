package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type pathParent interface {
	logger.Writer
	pathReady(*path)
	pathNotReady(*path)
	closePath(*path)
}

type pathOnDemandState int

const (
	pathOnDemandStateInitial pathOnDemandState = iota
	pathOnDemandStateWaitingReady
	pathOnDemandStateReady
	pathOnDemandStateClosing
)

type pathAPIPathsListRes struct {
	data  *defs.APIPathList
	paths map[string]*path
}

type pathAPIPathsListReq struct {
	res chan pathAPIPathsListRes
}

type pathAPIPathsGetRes struct {
	path *path
	data *defs.APIPath
	err  error
}

type pathAPIPathsGetReq struct {
	name string
	res  chan pathAPIPathsGetRes
}

type path struct {
	parentCtx         context.Context
	logLevel          conf.LogLevel
	rtspAddress       string
	readTimeout       conf.Duration
	writeTimeout      conf.Duration
	writeQueueSize    int
	udpMaxPayloadSize int
	conf              *conf.Path
	name              string
	matches           []string
	wg                *sync.WaitGroup
	externalCmdPool   *externalcmd.Pool
	parent            pathParent
	gopCache          bool

	ctx                            context.Context
	ctxCancel                      func()
	confMutex                      sync.RWMutex
	source                         defs.Source
	publisherQuery                 string
	stream                         *stream.Stream
	recorder                       *recorder.Recorder
	readyTime                      time.Time
	onUnDemandHook                 func(string)
	onNotReadyHook                 func()
	readers                        map[defs.Reader]struct{}
	describeRequestsOnHold         []defs.PathDescribeReq
	readerAddRequestsOnHold        []defs.PathAddReaderReq
	onDemandStaticSourceState      pathOnDemandState
	onDemandStaticSourceReadyTimer *time.Timer
	onDemandStaticSourceCloseTimer *time.Timer
	onDemandPublisherState         pathOnDemandState
	onDemandPublisherReadyTimer    *time.Timer
	onDemandPublisherCloseTimer    *time.Timer

	// in
	chReloadConf              chan *conf.Path
	chStaticSourceSetReady    chan defs.PathSourceStaticSetReadyReq
	chStaticSourceSetNotReady chan defs.PathSourceStaticSetNotReadyReq
	chDescribe                chan defs.PathDescribeReq
	chAddPublisher            chan defs.PathAddPublisherReq
	chRemovePublisher         chan defs.PathRemovePublisherReq
	chStartPublisher          chan defs.PathStartPublisherReq
	chStopPublisher           chan defs.PathStopPublisherReq
	chAddReader               chan defs.PathAddReaderReq
	chRemoveReader            chan defs.PathRemoveReaderReq
	chAPIPathsGet             chan pathAPIPathsGetReq

	// out
	done chan struct{}
}

func (pa *path) initialize() {
	ctx, ctxCancel := context.WithCancel(pa.parentCtx)

	pa.ctx = ctx
	pa.ctxCancel = ctxCancel
	pa.readers = make(map[defs.Reader]struct{})
	pa.onDemandStaticSourceReadyTimer = emptyTimer()
	pa.onDemandStaticSourceCloseTimer = emptyTimer()
	pa.onDemandPublisherReadyTimer = emptyTimer()
	pa.onDemandPublisherCloseTimer = emptyTimer()
	pa.chReloadConf = make(chan *conf.Path)
	pa.chStaticSourceSetReady = make(chan defs.PathSourceStaticSetReadyReq)
	pa.chStaticSourceSetNotReady = make(chan defs.PathSourceStaticSetNotReadyReq)
	pa.chDescribe = make(chan defs.PathDescribeReq)
	pa.chAddPublisher = make(chan defs.PathAddPublisherReq)
	pa.chRemovePublisher = make(chan defs.PathRemovePublisherReq)
	pa.chStartPublisher = make(chan defs.PathStartPublisherReq)
	pa.chStopPublisher = make(chan defs.PathStopPublisherReq)
	pa.chAddReader = make(chan defs.PathAddReaderReq)
	pa.chRemoveReader = make(chan defs.PathRemoveReaderReq)
	pa.chAPIPathsGet = make(chan pathAPIPathsGetReq)
	pa.done = make(chan struct{})

	pa.Log(logger.Debug, "created")

	pa.wg.Add(1)
	go pa.run()
}

func (pa *path) close() {
	pa.ctxCancel()
}

func (pa *path) wait() {
	<-pa.done
}

// Log implements logger.Writer.
func (pa *path) Log(level logger.Level, format string, args ...interface{}) {
	pa.parent.Log(level, "[path "+pa.name+"] "+format, args...)
}

func (pa *path) Name() string {
	return pa.name
}

func (pa *path) run() {
	defer close(pa.done)
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if pa.conf.HasStaticSource() {
		pa.source = &staticSourceHandler{
			conf:           pa.conf,
			logLevel:       pa.logLevel,
			readTimeout:    pa.readTimeout,
			writeTimeout:   pa.writeTimeout,
			writeQueueSize: pa.writeQueueSize,
			matches:        pa.matches,
			parent:         pa,
		}
		pa.source.(*staticSourceHandler).initialize()

		if !pa.conf.SourceOnDemand {
			pa.source.(*staticSourceHandler).start(false, "")
		}
	}

	onUnInitHook := hooks.OnInit(hooks.OnInitParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.ExternalCmdEnv(),
	})

	err := pa.runInner()

	// call before destroying context
	pa.parent.closePath(pa)

	pa.ctxCancel()

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherCloseTimer.Stop()

	onUnInitHook()

	for _, req := range pa.describeRequestsOnHold {
		req.Res <- defs.PathDescribeRes{Err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.readerAddRequestsOnHold {
		req.Res <- defs.PathAddReaderRes{Err: fmt.Errorf("terminated")}
	}

	if pa.stream != nil {
		pa.setNotReady()
	}

	if pa.source != nil {
		if source, ok := pa.source.(*staticSourceHandler); ok {
			if !pa.conf.SourceOnDemand || pa.onDemandStaticSourceState != pathOnDemandStateInitial {
				source.close("path is closing")
			}
		} else if source, ok := pa.source.(defs.Publisher); ok {
			source.Close()
		}
	}

	if pa.onUnDemandHook != nil {
		pa.onUnDemandHook("path destroyed")
	}

	pa.Log(logger.Debug, "destroyed: %v", err)
}

func (pa *path) runInner() error {
	for {
		select {
		case <-pa.onDemandStaticSourceReadyTimer.C:
			pa.doOnDemandStaticSourceReadyTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandStaticSourceCloseTimer.C:
			pa.doOnDemandStaticSourceCloseTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherReadyTimer.C:
			pa.doOnDemandPublisherReadyTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherCloseTimer.C:
			pa.doOnDemandPublisherCloseTimer()

		case newConf := <-pa.chReloadConf:
			pa.doReloadConf(newConf)

		case req := <-pa.chStaticSourceSetReady:
			pa.doSourceStaticSetReady(req)

		case req := <-pa.chStaticSourceSetNotReady:
			pa.doSourceStaticSetNotReady(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chDescribe:
			pa.doDescribe(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddPublisher:
			pa.doAddPublisher(req)

		case req := <-pa.chRemovePublisher:
			pa.doRemovePublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chStartPublisher:
			pa.doStartPublisher(req)

		case req := <-pa.chStopPublisher:
			pa.doStopPublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddReader:
			pa.doAddReader(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chRemoveReader:
			pa.doRemoveReader(req)

		case req := <-pa.chAPIPathsGet:
			pa.doAPIPathsGet(req)

		case <-pa.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

func (pa *path) doOnDemandStaticSourceReadyTimer() {
	for _, req := range pa.describeRequestsOnHold {
		req.Res <- defs.PathDescribeRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		req.Res <- defs.PathAddReaderRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.readerAddRequestsOnHold = nil

	pa.onDemandStaticSourceStop("timed out")
}

func (pa *path) doOnDemandStaticSourceCloseTimer() {
	pa.setNotReady()
	pa.onDemandStaticSourceStop("not needed by anyone")
}

func (pa *path) doOnDemandPublisherReadyTimer() {
	for _, req := range pa.describeRequestsOnHold {
		req.Res <- defs.PathDescribeRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		req.Res <- defs.PathAddReaderRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.readerAddRequestsOnHold = nil

	pa.onDemandPublisherStop("timed out")
}

func (pa *path) doOnDemandPublisherCloseTimer() {
	pa.onDemandPublisherStop("not needed by anyone")
}

func (pa *path) doReloadConf(newConf *conf.Path) {
	pa.confMutex.Lock()
	pa.conf = newConf
	pa.confMutex.Unlock()

	if pa.conf.HasStaticSource() {
		pa.source.(*staticSourceHandler).reloadConf(newConf)
	}

	if pa.conf.Record {
		if pa.stream != nil && pa.recorder == nil {
			pa.startRecording()
		}
	} else if pa.recorder != nil {
		pa.recorder.Close()
		pa.recorder = nil
	}
}

func (pa *path) doSourceStaticSetReady(req defs.PathSourceStaticSetReadyReq) {
	err := pa.setReady(req.Desc, req.GenerateRTPPackets)
	if err != nil {
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: err}
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		pa.onDemandStaticSourceReadyTimer.Stop()
		pa.onDemandStaticSourceReadyTimer = emptyTimer()
		pa.onDemandStaticSourceScheduleClose()
	}

	pa.consumeOnHoldRequests()

	req.Res <- defs.PathSourceStaticSetReadyRes{Stream: pa.stream}
}

func (pa *path) doSourceStaticSetNotReady(req defs.PathSourceStaticSetNotReadyReq) {
	pa.setNotReady()

	// send response before calling onDemandStaticSourceStop()
	// in order to avoid a deadlock due to staticSourceHandler.stop()
	close(req.Res)

	if pa.conf.HasOnDemandStaticSource() && pa.onDemandStaticSourceState != pathOnDemandStateInitial {
		pa.onDemandStaticSourceStop("an error occurred")
	}
}

func (pa *path) doDescribe(req defs.PathDescribeReq) {
	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- defs.PathDescribeRes{
			Redirect: pa.conf.SourceRedirect,
		}
		return
	}

	if pa.stream != nil {
		req.Res <- defs.PathDescribeRes{
			Stream: pa.stream,
		}
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart(req.AccessRequest.Query)
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateInitial {
			pa.onDemandPublisherStart(req.AccessRequest.Query)
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.conf.Fallback != "" {
		req.Res <- defs.PathDescribeRes{Redirect: pa.conf.Fallback}
		return
	}

	req.Res <- defs.PathDescribeRes{Err: defs.PathNoOnePublishingError{PathName: pa.name}}
}

func (pa *path) doRemovePublisher(req defs.PathRemovePublisherReq) {
	if pa.source == req.Author {
		pa.executeRemovePublisher()
	}
	close(req.Res)
}

func (pa *path) doAddPublisher(req defs.PathAddPublisherReq) {
	if pa.conf.Source != "publisher" {
		req.Res <- defs.PathAddPublisherRes{
			Err: fmt.Errorf("can't publish to path '%s' since 'source' is not 'publisher'", pa.name),
		}
		return
	}

	if pa.source != nil {
		if !pa.conf.OverridePublisher {
			req.Res <- defs.PathAddPublisherRes{Err: fmt.Errorf("someone is already publishing to path '%s'", pa.name)}
			return
		}

		pa.Log(logger.Info, "closing existing publisher")
		pa.source.(defs.Publisher).Close()
		pa.executeRemovePublisher()
	}

	pa.source = req.Author
	pa.publisherQuery = req.AccessRequest.Query

	req.Res <- defs.PathAddPublisherRes{Path: pa}
}

func (pa *path) doStartPublisher(req defs.PathStartPublisherReq) {
	if pa.source != req.Author {
		req.Res <- defs.PathStartPublisherRes{Err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	err := pa.setReady(req.Desc, req.GenerateRTPPackets)
	if err != nil {
		req.Res <- defs.PathStartPublisherRes{Err: err}
		return
	}

	req.Author.Log(logger.Info, "is publishing to path '%s', %s",
		pa.name,
		defs.MediasInfo(req.Desc.Medias))

	if pa.conf.HasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
		pa.onDemandPublisherReadyTimer.Stop()
		pa.onDemandPublisherReadyTimer = emptyTimer()
		pa.onDemandPublisherScheduleClose()
	}

	pa.consumeOnHoldRequests()

	req.Res <- defs.PathStartPublisherRes{Stream: pa.stream}
}

func (pa *path) doStopPublisher(req defs.PathStopPublisherReq) {
	if req.Author == pa.source && pa.stream != nil {
		pa.setNotReady()
	}
	close(req.Res)
}

func (pa *path) doAddReader(req defs.PathAddReaderReq) {
	if pa.stream != nil {
		pa.addReaderPost(req)
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart(req.AccessRequest.Query)
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateInitial {
			pa.onDemandPublisherStart(req.AccessRequest.Query)
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	req.Res <- defs.PathAddReaderRes{Err: defs.PathNoOnePublishingError{PathName: pa.name}}
}

func (pa *path) doRemoveReader(req defs.PathRemoveReaderReq) {
	if _, ok := pa.readers[req.Author]; ok {
		pa.executeRemoveReader(req.Author)
	}
	close(req.Res)

	if len(pa.readers) == 0 {
		if pa.conf.HasOnDemandStaticSource() {
			if pa.onDemandStaticSourceState == pathOnDemandStateReady {
				pa.onDemandStaticSourceScheduleClose()
			}
		} else if pa.conf.HasOnDemandPublisher() {
			if pa.onDemandPublisherState == pathOnDemandStateReady {
				pa.onDemandPublisherScheduleClose()
			}
		}
	}
}

func (pa *path) doAPIPathsGet(req pathAPIPathsGetReq) {
	req.res <- pathAPIPathsGetRes{
		data: &defs.APIPath{
			Name:     pa.name,
			ConfName: pa.conf.Name,
			Source: func() *defs.APIPathSourceOrReader {
				if pa.source == nil {
					return nil
				}
				v := pa.source.APISourceDescribe()
				return &v
			}(),
			Ready: pa.stream != nil,
			ReadyTime: func() *time.Time {
				if pa.stream == nil {
					return nil
				}
				v := pa.readyTime
				return &v
			}(),
			Tracks: func() []string {
				if pa.stream == nil {
					return []string{}
				}
				return defs.MediasToCodecs(pa.stream.Desc().Medias)
			}(),
			BytesReceived: func() uint64 {
				if pa.stream == nil {
					return 0
				}
				return pa.stream.BytesReceived()
			}(),
			BytesSent: func() uint64 {
				if pa.stream == nil {
					return 0
				}
				return pa.stream.BytesSent()
			}(),
			Readers: func() []defs.APIPathSourceOrReader {
				ret := []defs.APIPathSourceOrReader{}
				for r := range pa.readers {
					ret = append(ret, r.APIReaderDescribe())
				}
				return ret
			}(),
		},
	}
}

func (pa *path) SafeConf() *conf.Path {
	pa.confMutex.RLock()
	defer pa.confMutex.RUnlock()
	return pa.conf
}

func (pa *path) ExternalCmdEnv() externalcmd.Environment {
	_, port, _ := net.SplitHostPort(pa.rtspAddress)
	env := externalcmd.Environment{
		"MTX_PATH":  pa.name,
		"RTSP_PATH": pa.name, // deprecated
		"RTSP_PORT": port,
	}

	if len(pa.matches) > 1 {
		for i, ma := range pa.matches[1:] {
			env["G"+strconv.FormatInt(int64(i+1), 10)] = ma
		}
	}

	return env
}

func (pa *path) shouldClose() bool {
	return pa.conf.Regexp != nil &&
		pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequestsOnHold) == 0 &&
		len(pa.readerAddRequestsOnHold) == 0
}

func (pa *path) onDemandStaticSourceStart(query string) {
	pa.source.(*staticSourceHandler).start(true, query)

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceReadyTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandStartTimeout))

	pa.onDemandStaticSourceState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandStaticSourceScheduleClose() {
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandStaticSourceCloseTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandCloseAfter))

	pa.onDemandStaticSourceState = pathOnDemandStateClosing
}

func (pa *path) onDemandStaticSourceStop(reason string) {
	if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
		pa.onDemandStaticSourceCloseTimer.Stop()
		pa.onDemandStaticSourceCloseTimer = emptyTimer()
	}

	pa.onDemandStaticSourceState = pathOnDemandStateInitial

	pa.source.(*staticSourceHandler).stop(reason)
}

func (pa *path) onDemandPublisherStart(query string) {
	pa.onUnDemandHook = hooks.OnDemand(hooks.OnDemandParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.ExternalCmdEnv(),
		Query:           query,
	})

	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherReadyTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandStartTimeout))

	pa.onDemandPublisherState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandPublisherScheduleClose() {
	pa.onDemandPublisherCloseTimer.Stop()
	pa.onDemandPublisherCloseTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandCloseAfter))

	pa.onDemandPublisherState = pathOnDemandStateClosing
}

func (pa *path) onDemandPublisherStop(reason string) {
	if pa.onDemandPublisherState == pathOnDemandStateClosing {
		pa.onDemandPublisherCloseTimer.Stop()
		pa.onDemandPublisherCloseTimer = emptyTimer()
	}

	pa.onUnDemandHook(reason)
	pa.onUnDemandHook = nil

	pa.onDemandPublisherState = pathOnDemandStateInitial
}

func (pa *path) setReady(desc *description.Session, allocateEncoder bool) error {
	var err error
	pa.stream, err = stream.New(
		pa.writeQueueSize,
		pa.udpMaxPayloadSize,
		desc,
		allocateEncoder,
		logger.NewLimitedLogger(pa.source),
		pa.gopCache,
	)
	if err != nil {
		return err
	}

	if pa.conf.Record {
		pa.startRecording()
	}

	pa.readyTime = time.Now()

	pa.onNotReadyHook = hooks.OnReady(hooks.OnReadyParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.ExternalCmdEnv(),
		Desc:            pa.source.APISourceDescribe(),
		Query:           pa.publisherQuery,
	})

	pa.parent.pathReady(pa)

	return nil
}

func (pa *path) consumeOnHoldRequests() {
	for _, req := range pa.describeRequestsOnHold {
		req.Res <- defs.PathDescribeRes{
			Stream: pa.stream,
		}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		pa.addReaderPost(req)
	}
	pa.readerAddRequestsOnHold = nil
}

func (pa *path) setNotReady() {
	pa.parent.pathNotReady(pa)

	for r := range pa.readers {
		pa.executeRemoveReader(r)
		r.Close()
	}

	pa.onNotReadyHook()

	if pa.recorder != nil {
		pa.recorder.Close()
		pa.recorder = nil
	}

	if pa.stream != nil {
		pa.stream.Close()
		pa.stream = nil
	}
}

func (pa *path) startRecording() {
	pa.recorder = &recorder.Recorder{
		PathFormat:      pa.conf.RecordPath,
		Format:          pa.conf.RecordFormat,
		PartDuration:    time.Duration(pa.conf.RecordPartDuration),
		SegmentDuration: time.Duration(pa.conf.RecordSegmentDuration),
		PathName:        pa.name,
		Stream:          pa.stream,
		OnSegmentCreate: func(segmentPath string) {
			if pa.conf.RunOnRecordSegmentCreate != "" {
				env := pa.ExternalCmdEnv()
				env["MTX_SEGMENT_PATH"] = segmentPath

				pa.Log(logger.Info, "runOnRecordSegmentCreate command launched")
				externalcmd.NewCmd(
					pa.externalCmdPool,
					pa.conf.RunOnRecordSegmentCreate,
					false,
					env,
					nil)
			}
		},
		OnSegmentComplete: func(segmentPath string, segmentDuration time.Duration) {
			if pa.conf.RunOnRecordSegmentComplete != "" {
				env := pa.ExternalCmdEnv()
				env["MTX_SEGMENT_PATH"] = segmentPath
				env["MTX_SEGMENT_DURATION"] = strconv.FormatFloat(segmentDuration.Seconds(), 'f', -1, 64)

				pa.Log(logger.Info, "runOnRecordSegmentComplete command launched")
				externalcmd.NewCmd(
					pa.externalCmdPool,
					pa.conf.RunOnRecordSegmentComplete,
					false,
					env,
					nil)
			}
		},
		Parent: pa,
	}
	pa.recorder.Initialize()
}

func (pa *path) executeRemoveReader(r defs.Reader) {
	delete(pa.readers, r)
}

func (pa *path) executeRemovePublisher() {
	if pa.stream != nil {
		pa.setNotReady()
	}

	pa.source = nil
}

func (pa *path) addReaderPost(req defs.PathAddReaderReq) {
	if _, ok := pa.readers[req.Author]; ok {
		req.Res <- defs.PathAddReaderRes{
			Path:   pa,
			Stream: pa.stream,
		}
		return
	}

	if pa.conf.MaxReaders != 0 && len(pa.readers) >= pa.conf.MaxReaders {
		req.Res <- defs.PathAddReaderRes{Err: fmt.Errorf("maximum reader count reached")}
		return
	}

	pa.readers[req.Author] = struct{}{}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
			pa.onDemandStaticSourceState = pathOnDemandStateReady
			pa.onDemandStaticSourceCloseTimer.Stop()
			pa.onDemandStaticSourceCloseTimer = emptyTimer()
		}
	} else if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateClosing {
			pa.onDemandPublisherState = pathOnDemandStateReady
			pa.onDemandPublisherCloseTimer.Stop()
			pa.onDemandPublisherCloseTimer = emptyTimer()
		}
	}

	req.Res <- defs.PathAddReaderRes{
		Path:   pa,
		Stream: pa.stream,
	}
}

// reloadConf is called by pathManager.
func (pa *path) reloadConf(newConf *conf.Path) {
	select {
	case pa.chReloadConf <- newConf:
	case <-pa.ctx.Done():
	}
}

// staticSourceHandlerSetReady is called by staticSourceHandler.
func (pa *path) staticSourceHandlerSetReady(
	staticSourceHandlerCtx context.Context, req defs.PathSourceStaticSetReadyReq,
) {
	select {
	case pa.chStaticSourceSetReady <- req:

	case <-pa.ctx.Done():
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-staticSourceHandlerCtx.Done():
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}
	}
}

// staticSourceHandlerSetNotReady is called by staticSourceHandler.
func (pa *path) staticSourceHandlerSetNotReady(
	staticSourceHandlerCtx context.Context, req defs.PathSourceStaticSetNotReadyReq,
) {
	select {
	case pa.chStaticSourceSetNotReady <- req:

	case <-pa.ctx.Done():
		close(req.Res)

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-staticSourceHandlerCtx.Done():
		close(req.Res)
	}
}

// describe is called by a reader or publisher through pathManager.
func (pa *path) describe(req defs.PathDescribeReq) defs.PathDescribeRes {
	select {
	case pa.chDescribe <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return defs.PathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// addPublisher is called by a publisher through pathManager.
func (pa *path) addPublisher(req defs.PathAddPublisherReq) (defs.Path, error) {
	select {
	case pa.chAddPublisher <- req:
		res := <-req.Res
		return res.Path, res.Err
	case <-pa.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// RemovePublisher is called by a publisher.
func (pa *path) RemovePublisher(req defs.PathRemovePublisherReq) {
	req.Res = make(chan struct{})
	select {
	case pa.chRemovePublisher <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// StartPublisher is called by a publisher.
func (pa *path) StartPublisher(req defs.PathStartPublisherReq) (*stream.Stream, error) {
	req.Res = make(chan defs.PathStartPublisherRes)
	select {
	case pa.chStartPublisher <- req:
		res := <-req.Res
		return res.Stream, res.Err
	case <-pa.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// StopPublisher is called by a publisher.
func (pa *path) StopPublisher(req defs.PathStopPublisherReq) {
	req.Res = make(chan struct{})
	select {
	case pa.chStopPublisher <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// addReader is called by a reader through pathManager.
func (pa *path) addReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	select {
	case pa.chAddReader <- req:
		res := <-req.Res
		return res.Path, res.Stream, res.Err
	case <-pa.ctx.Done():
		return nil, nil, fmt.Errorf("terminated")
	}
}

// RemoveReader is called by a reader.
func (pa *path) RemoveReader(req defs.PathRemoveReaderReq) {
	req.Res = make(chan struct{})
	select {
	case pa.chRemoveReader <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// APIPathsGet is called by api.
func (pa *path) APIPathsGet(req pathAPIPathsGetReq) (*defs.APIPath, error) {
	req.res = make(chan pathAPIPathsGetRes)
	select {
	case pa.chAPIPathsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-pa.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}
