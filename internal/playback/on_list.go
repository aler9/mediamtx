package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

type listEntryDuration time.Duration

func (d listEntryDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Seconds())
}

type listEntry struct {
	Start    time.Time         `json:"start"`
	Duration listEntryDuration `json:"duration"`
	URL      string            `json:"url"`
}

func computeDurationAndConcatenate(
	recordFormat conf.RecordFormat,
	segments []*recordstore.Segment,
) ([]listEntry, error) {
	if recordFormat == conf.RecordFormatFMP4 {
		out := []listEntry{}
		var prevInit *fmp4.Init

		for _, seg := range segments {
			err := func() error {
				f, err := os.Open(seg.Fpath)
				if err != nil {
					return err
				}
				defer f.Close()

				init, err := segmentFMP4ReadInit(f)
				if err != nil {
					return err
				}

				_, err = f.Seek(0, io.SeekStart)
				if err != nil {
					return err
				}

				maxDuration, err := segmentFMP4ReadMaxDuration(f, init)
				if err != nil {
					return err
				}

				if len(out) != 0 && segmentFMP4CanBeConcatenated(
					prevInit,
					out[len(out)-1].Start.Add(time.Duration(out[len(out)-1].Duration)),
					init,
					seg.Start,
					false) {
					prevStart := out[len(out)-1].Start
					curEnd := seg.Start.Add(maxDuration)
					out[len(out)-1].Duration = listEntryDuration(curEnd.Sub(prevStart))
				} else {
					out = append(out, listEntry{
						Start:    seg.Start,
						Duration: listEntryDuration(maxDuration),
					})
				}

				prevInit = init

				return nil
			}()
			if err != nil {
				return nil, err
			}
		}

		return out, nil
	}

	return nil, fmt.Errorf("MPEG-TS format is not supported yet")
}

func (s *Server) onList(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !s.doAuth(ctx, pathName) {
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	segments, err := recordstore.FindSegments(pathConf, pathName)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	entries, err := computeDurationAndConcatenate(pathConf.RecordFormat, segments)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	var scheme string
	if s.Encryption {
		scheme = "https"
	} else {
		scheme = "http"
	}

	for i := range entries {
		v := url.Values{}
		v.Add("path", pathName)
		v.Add("start", entries[i].Start.Format(time.RFC3339Nano))
		v.Add("duration", strconv.FormatFloat(time.Duration(entries[i].Duration).Seconds(), 'f', -1, 64))
		u := &url.URL{
			Scheme:   scheme,
			Host:     ctx.Request.Host,
			Path:     "/get",
			RawQuery: v.Encode(),
		}
		entries[i].URL = u.String()
	}

	ctx.JSON(http.StatusOK, entries)
}
