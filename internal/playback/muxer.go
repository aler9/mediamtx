package playback

type muxer interface {
	writeInit(init []byte)
	setTrack(trackID int)
	writeSample(dts int64, ptsOffset int32, isNonSyncSample bool, payload []byte) error
	writeFinalDTS(dts int64)
	flush() error
}
