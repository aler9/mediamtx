package hls

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	stream, err := stream.New(
		512,
		1460,
		&description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		}}},
		true,
		test.NilLogger,
		false,
	)
	require.NoError(t, err)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})

	m := &gohlslib.Muxer{}

	err = FromStream(stream, l, m)
	require.Equal(t, ErrNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	stream, err := stream.New(
		512,
		1460,
		&description.Session{Medias: []*description.Media{
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{}},
			},
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.VP8{}},
			},
			{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			},
		}},
		true,
		test.NilLogger,
		false,
	)
	require.NoError(t, err)

	m := &gohlslib.Muxer{}

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		switch n {
		case 0:
			require.Equal(t, "skipping track 2 (VP8)", fmt.Sprintf(format, args...))
		case 1:
			require.Equal(t, "skipping track 3 (MPEG-1/2 Audio)", fmt.Sprintf(format, args...))
		}
		n++
	})

	err = FromStream(stream, l, m)
	require.NoError(t, err)
	defer stream.RemoveReader(l)

	require.Equal(t, 2, n)
}
