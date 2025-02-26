package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

type listEntryDuration time.Duration

func (d listEntryDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Seconds())
}

type parsedSegment struct {
	start    time.Time
	init     *fmp4.Init
	duration time.Duration
}

func parseSegment(seg *recordstore.Segment) (*parsedSegment, error) {
	f, err := os.Open(seg.Fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	init, duration, err := segmentFMP4ReadHeader(f)
	if err != nil {
		return nil, err
	}

	// if duration is not present in the header, compute it
	// by parsing each part
	if duration == 0 {
		duration, err = segmentFMP4ReadDurationFromParts(f, init)
		if err != nil {
			return nil, err
		}
	}

	return &parsedSegment{
		start:    seg.Start,
		init:     init,
		duration: duration,
	}, nil
}

func parseSegments(segments []*recordstore.Segment) ([]*parsedSegment, error) {
	parsed := make([]*parsedSegment, len(segments))
	ch := make(chan error)

	// process segments in parallel.
	// parallel random access should improve performance in most cases.
	// ref: https://pkolaczk.github.io/disk-parallelism/
	for i, seg := range segments {
		go func(i int, seg *recordstore.Segment) {
			var err error
			parsed[i], err = parseSegment(seg)
			ch <- err
		}(i, seg)
	}

	var err error

	for range segments {
		err2 := <-ch
		if err2 != nil {
			err = err2
		}
	}

	return parsed, err
}

type listEntry struct {
	Start    time.Time         `json:"start"`
	Duration listEntryDuration `json:"duration"`
	URL      string            `json:"url"`
}

func concatenateSegments(parsed []*parsedSegment) []listEntry {
	out := []listEntry{}
	var prevInit *fmp4.Init

	for _, parsed := range parsed {
		if len(out) != 0 && segmentFMP4CanBeConcatenated(
			prevInit,
			out[len(out)-1].Start.Add(time.Duration(out[len(out)-1].Duration)),
			parsed.init,
			parsed.start) {
			prevStart := out[len(out)-1].Start
			curEnd := parsed.start.Add(parsed.duration)
			out[len(out)-1].Duration = listEntryDuration(curEnd.Sub(prevStart))
		} else {
			out = append(out, listEntry{
				Start:    parsed.start,
				Duration: listEntryDuration(parsed.duration),
			})
		}

		prevInit = parsed.init
	}

	return out
}

func parseAndConcatenate(
	recordFormat conf.RecordFormat,
	segments []*recordstore.Segment,
) ([]listEntry, error) {
	if recordFormat == conf.RecordFormatFMP4 {
		parsed, err := parseSegments(segments)
		if err != nil {
			return nil, err
		}

		out := concatenateSegments(parsed)
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

	var start *time.Time
	rawStart := ctx.Query("start")
	if rawStart != "" {
		var tmp time.Time
		tmp, err = time.Parse(time.RFC3339, rawStart)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
			return
		}
		start = &tmp
	}

	var end *time.Time
	rawEnd := ctx.Query("end")
	if rawEnd != "" {
		var tmp time.Time
		tmp, err = time.Parse(time.RFC3339, rawEnd)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid end: %w", err))
			return
		}
		end = &tmp
	}

	segments, err := recordstore.FindSegments(pathConf, pathName, start, end)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	entries, err := parseAndConcatenate(pathConf.RecordFormat, segments)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	if start != nil {
		firstEntry := entries[0]

		// when start is placed in a gap between the first and second segment,
		// or when there's no second segment,
		// the first segment is erroneously included with a negative duration.
		// remove it.
		if firstEntry.Start.Add(time.Duration(firstEntry.Duration)).Before(*start) {
			entries = entries[1:]

			if len(entries) == 0 {
				s.writeError(ctx, http.StatusNotFound, recordstore.ErrNoSegmentsFound)
				return
			}
		} else if firstEntry.Start.Before(*start) {
			entries[0].Duration -= listEntryDuration(start.Sub(firstEntry.Start))
			entries[0].Start = *start
		}
	}

	if end != nil {
		lastEntry := entries[len(entries)-1]
		if lastEntry.Start.Add(time.Duration(lastEntry.Duration)).After(*end) {
			entries[len(entries)-1].Duration = listEntryDuration(end.Sub(lastEntry.Start))
		}
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
