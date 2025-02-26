package playback

import (
	"io"
	"os"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/test"
)

func writeBenchInit(f io.WriteSeeker) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &fmp4.CodecH264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
			{
				ID:        2,
				TimeScale: 90000,
				Codec: &fmp4.CodecMPEG4Audio{
					Config: mpeg4audio.Config{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	err := init.Marshal(f)
	if err != nil {
		panic(err)
	}

	_, err = f.Write([]byte{
		0x00, 0x00, 0x00, 0x10, 'm', 'o', 'o', 'f',
	})
	if err != nil {
		panic(err)
	}
}

func BenchmarkFMP4ReadHeader(b *testing.B) {
	f, err := os.CreateTemp(os.TempDir(), "mediamtx-playback-fmp4-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())

	writeBenchInit(f)
	f.Close()

	for n := 0; n < b.N; n++ {
		func() {
			f, err = os.Open(f.Name())
			if err != nil {
				panic(err)
			}
			defer f.Close()

			_, _, err = segmentFMP4ReadHeader(f)
			if err != nil {
				panic(err)
			}
		}()
	}
}
