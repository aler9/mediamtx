package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestOnList(t *testing.T) {
	for _, ca := range []string{
		"unfiltered",
		"filtered",
		"filtered and gap",
		"different init",
		"start after duration",
		"start before first",
	} {
		t.Run(ca, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-playback")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			switch ca {
			case "unfiltered", "filtered", "start before first":
				writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
				writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))
				writeSegment2(t, filepath.Join(dir, "mypath", "2009-11-07_11-23-02-500000.mp4"))

			case "filtered and gap":
				writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
				writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-24-02-500000.mp4"))

			case "different init":
				writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
				writeSegment3(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))

			case "start after duration":
				writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
			}

			s := &Server{
				Address:     "127.0.0.1:9996",
				ReadTimeout: conf.Duration(10 * time.Second),
				PathConfs: map[string]*conf.Path{
					"mypath": {
						Name:       "mypath",
						RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
					},
				},
				AuthManager: test.NilAuthManager,
				Parent:      test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:9996/list?start=")
			require.NoError(t, err)

			v := url.Values{}
			v.Set("path", "mypath")

			switch ca {
			case "filtered":
				v.Set("start", time.Date(2008, 11, 0o7, 11, 22, 1, 500000000, time.Local).Format(time.RFC3339Nano))
				v.Set("end", time.Date(2009, 11, 0o7, 11, 23, 4, 500000000, time.Local).Format(time.RFC3339Nano))

			case "filtered and gap":
				v.Set("start", time.Date(2008, 11, 0o7, 11, 23, 20, 500000000, time.Local).Format(time.RFC3339Nano))
				v.Set("end", time.Date(2009, 11, 0o7, 11, 23, 4, 500000000, time.Local).Format(time.RFC3339Nano))

			case "start after duration":
				v.Set("start", time.Date(2010, 11, 0o7, 11, 23, 20, 500000000, time.Local).Format(time.RFC3339Nano))

			case "start before first":
				v.Set("start", time.Date(2007, 11, 0o7, 11, 23, 20, 500000000, time.Local).Format(time.RFC3339Nano))
			}

			u.RawQuery = v.Encode()

			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			if ca == "start after duration" {
				require.Equal(t, http.StatusNotFound, res.StatusCode)
				return
			}

			require.Equal(t, http.StatusOK, res.StatusCode)

			var out interface{}
			err = json.NewDecoder(res.Body).Decode(&out)
			require.NoError(t, err)

			switch ca {
			case "unfiltered", "start before first":
				require.Equal(t, []interface{}{
					map[string]interface{}{
						"duration": float64(65),
						"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=65&path=mypath&start=" +
							url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
					map[string]interface{}{
						"duration": float64(3),
						"start":    time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=3&path=mypath&start=" +
							url.QueryEscape(time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
				}, out)

			case "filtered":
				require.Equal(t, []interface{}{
					map[string]interface{}{
						"duration": float64(64),
						"start":    time.Date(2008, 11, 0o7, 11, 22, 1, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=64&path=mypath&start=" +
							url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 1, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
					map[string]interface{}{
						"duration": float64(2),
						"start":    time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=2&path=mypath&start=" +
							url.QueryEscape(time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
				}, out)

			case "filtered and gap":
				require.Equal(t, []interface{}{
					map[string]interface{}{
						"duration": float64(3),
						"start":    time.Date(2008, 11, 0o7, 11, 24, 2, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=3&path=mypath&start=" +
							url.QueryEscape(time.Date(2008, 11, 0o7, 11, 24, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
				}, out)

			case "different init":
				require.Equal(t, []interface{}{
					map[string]interface{}{
						"duration": float64(62),
						"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=62&path=mypath&start=" +
							url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
					map[string]interface{}{
						"duration": float64(1),
						"start":    time.Date(2008, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
						"url": "http://localhost:9996/get?duration=1&path=mypath&start=" +
							url.QueryEscape(time.Date(2008, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
					},
				}, out)
			}
		})
	}
}

func writeDuration(f io.ReadWriteSeeker, d time.Duration) error {
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// check and skip ftyp header and content

	buf := make([]byte, 8)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = f.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return err
	}

	// check and skip moov header

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	moovPos, err := f.Seek(8, io.SeekCurrent)
	if err != nil {
		return err
	}

	var mvhd mp4.Mvhd
	_, err = mp4.Unmarshal(f, uint64(moovSize-8), &mvhd, mp4.Context{})
	if err != nil {
		return err
	}

	mvhd.DurationV0 = uint32(d / time.Millisecond)

	_, err = f.Seek(moovPos, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = mp4.Marshal(f, &mvhd, mp4.Context{})
	if err != nil {
		return err
	}

	return nil
}

func TestOnListCachedDuration(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	func() {
		var f *os.File
		f, err = os.Create(filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
		require.NoError(t, err)
		defer f.Close()

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
			},
		}

		err = init.Marshal(f)
		require.NoError(t, err)

		err = writeDuration(f, 50*time.Second)
		require.NoError(t, err)
	}()

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.Duration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:       "mypath",
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/list")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var out interface{}
	err = json.NewDecoder(res.Body).Decode(&out)
	require.NoError(t, err)

	require.Equal(t, []interface{}{
		map[string]interface{}{
			"duration": float64(50),
			"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
			"url": "http://localhost:9996/get?duration=50&path=mypath&start=" +
				url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano)),
		},
	}, out)
}
