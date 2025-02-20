package mpegts

import (
	"fmt"
	"testing"

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

	err = FromStream(stream, l, nil, nil, 0)
	require.Equal(t, errNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	stream, err := stream.New(
		512,
		1460,
		&description.Session{Medias: []*description.Media{
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{}},
			},
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.VP8{}},
			},
		}},
		true,
		test.NilLogger,
		false,
	)
	require.NoError(t, err)

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		if n == 0 {
			require.Equal(t, "skipping track 2 (VP8)", fmt.Sprintf(format, args...))
		}
		n++
	})

	err = FromStream(stream, l, nil, nil, 0)
	require.NoError(t, err)
	defer stream.RemoveReader(l)

	require.Equal(t, 1, n)
}
