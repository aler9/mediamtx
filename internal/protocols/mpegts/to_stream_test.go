package mpegts

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	var buf bytes.Buffer
	mux := astits.NewMuxer(context.Background(), &buf)

	err := mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 122,
		StreamType:    astits.StreamTypeDTSAudio,
	})
	require.NoError(t, err)

	mux.SetPCRPID(122)

	_, err = mux.WriteTables()
	require.NoError(t, err)

	r, err := mpegts.NewReader(&buf)
	require.NoError(t, err)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})
	_, err = ToStream(r, nil, l)
	require.Equal(t, errNoSupportedCodecs, err)
}

func TestToStreamSkipUnsupportedTracks(t *testing.T) {
	var buf bytes.Buffer
	mux := astits.NewMuxer(context.Background(), &buf)

	err := mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 122,
		StreamType:    astits.StreamTypeDTSAudio,
	})
	require.NoError(t, err)

	err = mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 123,
		StreamType:    astits.StreamTypeH264Video,
	})
	require.NoError(t, err)

	mux.SetPCRPID(122)

	_, err = mux.WriteTables()
	require.NoError(t, err)

	r, err := mpegts.NewReader(&buf)
	require.NoError(t, err)

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		if n == 0 {
			require.Equal(t, "skipping track 1 (unsupported codec)", fmt.Sprintf(format, args...))
		}
		n++
	})

	_, err = ToStream(r, nil, l)
	require.NoError(t, err)
}
