<h1 align="center">
  <img src="logo.png" alt="MediaMTX / rtsp-simple-server">

  <br>
  <br>

  [![Test](https://github.com/bluenviron/mediamtx/workflows/code_test/badge.svg)](https://github.com/bluenviron/mediamtx/actions?query=workflow:code_test)
  [![Lint](https://github.com/bluenviron/mediamtx/workflows/code_lint/badge.svg)](https://github.com/bluenviron/mediamtx/actions?query=workflow:code_lint)
  [![CodeCov](https://codecov.io/gh/bluenviron/mediamtx/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/mediamtx/tree/main)
  [![Release](https://img.shields.io/github/v/release/bluenviron/mediamtx)](https://github.com/bluenviron/mediamtx/releases)
  [![Docker Hub](https://img.shields.io/badge/docker-bluenviron/mediamtx-blue)](https://hub.docker.com/r/bluenviron/mediamtx)
  [![API Documentation](https://img.shields.io/badge/api-documentation-blue)](https://bluenviron.github.io/mediamtx)
</h1>

<br>

_MediaMTX_ is a ready-to-use and zero-dependency real-time media server and media proxy that allows to publish, read, proxy, record and playback video and audio streams. It has been conceived as a "media router" that routes media streams from one end to the other.

Live streams can be published to the server with:

|protocol|variants|video codecs|audio codecs|
|--------|--------|------------|------------|
|[SRT clients](#srt-clients)||H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3|
|[SRT cameras and servers](#srt-cameras-and-servers)||H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3|
|[WebRTC clients](#webrtc-clients)|WHIP|AV1, VP9, VP8, [H265](#supported-browsers), H264|Opus, G722, G711 (PCMA, PCMU)|
|[WebRTC servers](#webrtc-servers)|WHEP|AV1, VP9, VP8, [H265](#supported-browsers), H264|Opus, G722, G711 (PCMA, PCMU)|
|[RTSP clients](#rtsp-clients)|UDP, TCP, RTSPS|AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec|
|[RTSP cameras and servers](#rtsp-cameras-and-servers)|UDP, UDP-Multicast, TCP, RTSPS|AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec|
|[RTMP clients](#rtmp-clients)|RTMP, RTMPS, Enhanced RTMP|AV1, VP9, H265, H264|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM|
|[RTMP cameras and servers](#rtmp-cameras-and-servers)|RTMP, RTMPS, Enhanced RTMP|AV1, VP9, H265, H264|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM|
|[HLS cameras and servers](#hls-cameras-and-servers)|Low-Latency HLS, MP4-based HLS, legacy HLS|AV1, VP9, [H265](#supported-browsers-1), H264|Opus, MPEG-4 Audio (AAC)|
|[UDP/MPEG-TS](#udpmpeg-ts)|Unicast, broadcast, multicast|H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3|
|[Raspberry Pi Cameras](#raspberry-pi-cameras)||H264||

Live streams can be read from the server with:

|protocol|variants|video codecs|audio codecs|
|--------|--------|------------|------------|
|[SRT](#srt)||H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3|
|[WebRTC](#webrtc)|WHEP|AV1, VP9, VP8, [H265](#supported-browsers), H264|Opus, G722, G711 (PCMA, PCMU)|
|[RTSP](#rtsp)|UDP, UDP-Multicast, TCP, RTSPS|AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec|
|[RTMP](#rtmp)|RTMP, RTMPS, Enhanced RTMP|H264|MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3)|
|[HLS](#hls)|Low-Latency HLS, MP4-based HLS, legacy HLS|AV1, VP9, [H265](#supported-browsers-1), H264|Opus, MPEG-4 Audio (AAC)|

Live streams be recorded and played back with:

|format|video codecs|audio codecs|
|------|------------|------------|
|[fMP4](#record-streams-to-disk)|AV1, VP9, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM|
|[MPEG-TS](#record-streams-to-disk)|H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video|Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3|

**Features**

* Publish live streams to the server
* Read live streams from the server
* Streams are automatically converted from a protocol to another
* Serve multiple streams at once in separate paths
* Record streams to disk
* Playback recorded streams
* Authenticate users
* Redirect readers to other RTSP servers (load balancing)
* Control the server through the Control API
* Reload the configuration without disconnecting existing clients (hot reloading)
* Read Prometheus-compatible metrics
* Run hooks (external commands) when clients connect, disconnect, read or publish streams
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

**Note about rtsp-simple-server**

_rtsp-simple-server_ has been rebranded as _MediaMTX_. The reason is pretty obvious: this project started as a RTSP server but has evolved into a much more versatile product that is not tied to the RTSP protocol anymore. Nothing will change regarding license, features and backward compatibility.

## Table of contents

* [Installation](#installation)
  * [Standalone binary](#standalone-binary)
  * [Docker image](#docker-image)
  * [Arch Linux package](#arch-linux-package)
  * [OpenWrt binary](#openwrt-binary)
* [Basic usage](#basic-usage)
* [Publish to the server](#publish-to-the-server)
  * [By software](#by-software)
    * [FFmpeg](#ffmpeg)
    * [GStreamer](#gstreamer)
    * [OBS Studio](#obs-studio)
    * [OpenCV](#opencv)
    * [Unity](#unity)
    * [Web browsers](#web-browsers)
  * [By device](#by-device)
    * [Generic webcam](#generic-webcam)
    * [Raspberry Pi Cameras](#raspberry-pi-cameras)
  * [By protocol](#by-protocol)
    * [SRT clients](#srt-clients)
    * [SRT cameras and servers](#srt-cameras-and-servers)
    * [WebRTC clients](#webrtc-clients)
    * [WebRTC servers](#webrtc-servers)
    * [RTSP clients](#rtsp-clients)
    * [RTSP cameras and servers](#rtsp-cameras-and-servers)
    * [RTMP clients](#rtmp-clients)
    * [RTMP cameras and servers](#rtmp-cameras-and-servers)
    * [HLS cameras and servers](#hls-cameras-and-servers)
    * [UDP/MPEG-TS](#udpmpeg-ts)
* [Read from the server](#read-from-the-server)
  * [By software](#by-software-1)
    * [FFmpeg](#ffmpeg-1)
    * [GStreamer](#gstreamer-1)
    * [VLC](#vlc)
    * [Unity](#unity-1)
    * [Web browsers](#web-browsers-1)
  * [By protocol](#by-protocol-1)
    * [SRT](#srt)
    * [WebRTC](#webrtc)
    * [RTSP](#rtsp)
    * [RTMP](#rtmp)
    * [HLS](#hls)
* [Other features](#other-features)
  * [Configuration](#configuration)
  * [Authentication](#authentication)
    * [Internal](#internal)
    * [HTTP-based](#http-based)
    * [JWT-based](#jwt-based)
  * [Encrypt the configuration](#encrypt-the-configuration)
  * [Remuxing, re-encoding, compression](#remuxing-re-encoding-compression)
  * [Record streams to disk](#record-streams-to-disk)
  * [Playback recorded streams](#playback-recorded-streams)
  * [Forward streams to other servers](#forward-streams-to-other-servers)
  * [Proxy requests to other servers](#proxy-requests-to-other-servers)
  * [On-demand publishing](#on-demand-publishing)
  * [Start on boot](#start-on-boot)
    * [Linux](#linux)
    * [OpenWrt](#openwrt)
    * [Windows](#windows)
  * [Hooks](#hooks)
  * [Control API](#control-api)
  * [Metrics](#metrics)
  * [pprof](#pprof)
  * [SRT-specific features](#srt-specific-features)
    * [Standard stream ID syntax](#standard-stream-id-syntax)
  * [WebRTC-specific features](#webrtc-specific-features)
    * [Authenticating with WHIP/WHEP](#authenticating-with-whipwhep)
    * [Solving WebRTC connectivity issues](#solving-webrtc-connectivity-issues)
    * [Supported browsers](#supported-browsers)
  * [HLS-specific features](#hls-specific-features)
    * [Supported browsers](#supported-browsers-1)
  * [RTSP-specific features](#rtsp-specific-features)
    * [Transport protocols](#transport-protocols)
    * [Encryption](#encryption)
    * [Corrupted frames](#corrupted-frames)
  * [RTMP-specific features](#rtmp-specific-features)
    * [Encryption](#encryption-1)
* [Compile from source](#compile-from-source)
  * [Standard](#standard)
  * [OpenWrt](#openwrt-1)
  * [Custom libcamera](#custom-libcamera)
  * [Cross compile](#cross-compile)
  * [Compile for all supported platforms](#compile-for-all-supported-platforms)
* [License](#license)
* [Specifications](#specifications)
* [Related projects](#related-projects)

## Installation

There are several installation methods available: standalone binary, Docker image, Arch Linux package and OpenWrt binary.

### Standalone binary

1. Download and extract a standalone binary from the [release page](https://github.com/bluenviron/mediamtx/releases) that corresponds to your operating system and architecture.

2. Start the server:

   ```sh
   ./mediamtx
   ```

### Docker image

Download and launch the image:

```
docker run --rm -it --network=host bluenviron/mediamtx:latest
```

Available images:

|name|FFmpeg included|RPI Camera support|
|----|---------------|------------------|
|bluenviron/mediamtx:latest|:x:|:x:|
|bluenviron/mediamtx:latest-ffmpeg|:heavy_check_mark:|:x:|
|bluenviron/mediamtx:latest-rpi|:x:|:heavy_check_mark:|
|bluenviron/mediamtx:latest-ffmpeg-rpi|:heavy_check_mark:|:heavy_check_mark:|

The `--network=host` flag is mandatory for RTSP to work, since Docker can change the source port of UDP packets for routing reasons, and this doesn't allow the server to identify the senders of the packets. This issue can be avoided by disabling the RTSP UDP transport protocol:

```
docker run --rm -it \
-e MTX_RTSPTRANSPORTS=tcp \
-e MTX_WEBRTCADDITIONALHOSTS=192.168.x.x \
-p 8554:8554 \
-p 1935:1935 \
-p 8888:8888 \
-p 8889:8889 \
-p 8890:8890/udp \
-p 8189:8189/udp \
bluenviron/mediamtx
```

set `MTX_WEBRTCADDITIONALHOSTS` to your local IP address.

### Arch Linux package

If you are running the Arch Linux distribution, run:

```sh
git clone https://aur.archlinux.org/mediamtx.git
cd mediamtx
makepkg -si
```

### OpenWrt binary

If the architecture of the OpenWrt device is amd64, armv6, armv7 or arm64, use the [standalone binary method](#standalone-binary) and download a Linux binary that corresponds to your architecture.

Otherwise, [compile the server from source](#openwrt-1).

## Basic usage

1. Publish a stream. For instance, you can publish a video/audio file with _FFmpeg_:

   ```sh
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```sh
   gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream filesrc location=file.mp4 \
   ! qtdemux name=d d.video_0 ! queue ! s.sink_0 d.audio_0 ! queue ! s.sink_1
   ```

2. Open the stream. For instance, you can open the stream with _VLC_:

   ```sh
   vlc --network-caching=50 rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```sh
   gst-play-1.0 rtsp://localhost:8554/mystream
   ```

   or _FFmpeg_:

   ```sh
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```

## Publish to the server

### By software

#### FFmpeg

FFmpeg can publish a stream to the server in multiple ways (SRT client, SRT server, RTSP client, RTMP client, UDP/MPEG-TS, WebRTC with WHIP). The recommended one consists in publishing as a [RTSP client](#rtsp-clients):

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
```

The RTSP protocol supports multiple underlying transport protocols, each with its own characteristics (see [RTSP-specific features](#rtsp-specific-features)). You can set the transport protocol by using the `rtsp_transport` flag, for instance, in order to use TCP:

```sh
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp -rtsp_transport tcp rtsp://localhost:8554/mystream
```

The resulting stream will be available in path `/mystream`.

#### GStreamer

GStreamer can publish a stream to the server in multiple ways (SRT client, SRT server, RTSP client, RTMP client, UDP/MPEG-TS, WebRTC with WHIP). The recommended one consists in publishing as a [RTSP client](#rtsp-clients):

```sh
gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream \
filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! queue ! s.sink_0 \
d.audio_0 ! queue ! s.sink_1
```

If the stream is video only:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream
```

The RTSP protocol supports multiple underlying transport protocols, each with its own characteristics (see [RTSP-specific features](#rtsp-specific-features)). You can set the transport protocol by using the `protocols` flag:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink protocols=tcp name=s location=rtsp://localhost:8554/mystream
```

The resulting stream will be available in path `/mystream`.

GStreamer can also publish a stream by using the [WebRTC / WHIP protocol](#webrtc). Make sure that GStreamer version is at least 1.22, and that if the codec is H264, the profile is baseline. Use the `whipclientsink` element:

```
gst-launch-1.0 videotestsrc \
! video/x-raw,width=1920,height=1080,format=I420 \
! x264enc speed-preset=ultrafast bitrate=2000 \
! video/x-h264,profile=baseline \
! whipclientsink signaller::whip-endpoint=http://localhost:8889/mystream/whip
```

#### OBS Studio

OBS Studio can publish to the server in multiple ways (SRT client, RTMP client, WebRTC client). The recommended one consists in publishing as a [RTMP client](#rtmp-clients). In `Settings -> Stream` (or in the Auto-configuration Wizard), use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream`

If credentials are in use, use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream?user=myuser&pass=mypass`

Save the configuration and click `Start streaming`.

If you want to generate a stream that can be read with WebRTC, open `Settings -> Output -> Recording` and use the following parameters:

* FFmpeg output type: `Output to URL`
* File path or URL: `rtsp://localhost:8554/mystream`
* Container format: `rtsp`
* Check `show all codecs (even if potentically incompatible)`
* Video encoder: `h264_nvenc (libx264)`
* Video encoder settings (if any): `bf=0`
* Audio track: `1`
* Audio encoder: `libopus`

Then use the button `Start Recording` (instead of `Start Streaming`) to start streaming.

Latest versions of OBS Studio can publish to the server with the [WebRTC / WHIP protocol](#webrtc). Use the following parameters:

* Service: `WHIP`
* Server: `http://localhost:8889/mystream/whip`
* Bearer Token: `myuser:mypass` (if internal authentication is enabled) or JWT (if JWT-based authentication is enabled)

Save the configuration and click `Start streaming`.

The resulting stream will be available in path `/mystream`.

#### OpenCV

Software which uses the OpenCV library can publish to the server through its GStreamer plugin, as a [RTSP client](#rtsp-clients). It must be compiled with GStreamer support, by following this procedure:

```sh
sudo apt install -y libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-ugly gstreamer1.0-rtsp python3-dev python3-numpy
git clone --depth=1 -b 4.5.4 https://github.com/opencv/opencv
cd opencv
mkdir build && cd build
cmake -D CMAKE_INSTALL_PREFIX=/usr -D WITH_GSTREAMER=ON ..
make -j$(nproc)
sudo make install
```

You can check that OpenCV has been installed correctly by running:

```sh
python3 -c 'import cv2; print(cv2.getBuildInformation())'
```

Check that the output contains `GStreamer: YES`.

Videos can be published with `cv2.VideoWriter`:

```python
from datetime import datetime
from time import sleep, time

import cv2
import numpy as np

fps = 15
width = 800
height = 600
colors = [
    (0, 0, 255),
    (255, 0, 0),
    (0, 255, 0),
]

out = cv2.VideoWriter('appsrc ! videoconvert' + \
    ' ! video/x-raw,format=I420' + \
    ' ! x264enc speed-preset=ultrafast bitrate=600 key-int-max=' + str(fps * 2) + \
    ' ! video/x-h264,profile=baseline' + \
    ' ! rtspclientsink location=rtsp://localhost:8554/mystream',
    cv2.CAP_GSTREAMER, 0, fps, (width, height), True)
if not out.isOpened():
    raise Exception("can't open video writer")

curcolor = 0
start = time()

while True:
    frame = np.zeros((height, width, 3), np.uint8)

    # create a rectangle
    color = colors[curcolor]
    curcolor += 1
    curcolor %= len(colors)
    for y in range(0, int(frame.shape[0] / 2)):
        for x in range(0, int(frame.shape[1] / 2)):
            frame[y][x] = color

    out.write(frame)
    print("%s frame written to the server" % datetime.now())

    now = time()
    diff = (1 / fps) - now - start
    if diff > 0:
        sleep(diff)
    start = now
```

The resulting stream will be available in path `/mystream`.

#### Unity

Software written with the Unity Engine can publish a stream to the server by using the [WebRTC protocol](#webrtc).

Create a new Unity project or open an existing open.

Open _Window -> Package Manager_, click on the plus sign, _Add Package by name..._ and insert `com.unity.webrtc`. Wait for the package to be installed.

In the _Project_ window, under `Assets`, create a new C# Script called `WebRTCPublisher.cs` with this content:

```cs
using System.Collections;
using UnityEngine;
using Unity.WebRTC;
using UnityEngine.Networking;

public class WebRTCPublisher : MonoBehaviour
{
    public string url = "http://localhost:8889/unity/whip";
    public int videoWidth = 1280;
    public int videoHeight = 720;

    private RTCPeerConnection pc;
    private MediaStream videoStream;

    void Start()
    {
        pc = new RTCPeerConnection();
        Camera sourceCamera = gameObject.GetComponent<Camera>();
        videoStream = sourceCamera.CaptureStream(videoWidth, videoHeight);
        foreach (var track in videoStream.GetTracks())
        {
            pc.AddTrack(track);
        }

        StartCoroutine(WebRTC.Update());
        StartCoroutine(createOffer());
    }

    private IEnumerator createOffer()
    {
        var op = pc.CreateOffer();
        yield return op;
        if (op.IsError) {
            Debug.LogError("CreateOffer() failed");
            yield break;
        }

        yield return setLocalDescription(op.Desc);
    }

    private IEnumerator setLocalDescription(RTCSessionDescription offer)
    {
        var op = pc.SetLocalDescription(ref offer);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetLocalDescription() failed");
            yield break;
        }

        yield return postOffer(offer);
    }

    private IEnumerator postOffer(RTCSessionDescription offer)
    {
        var content = new System.Net.Http.StringContent(offer.sdp);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/sdp");
        var client = new System.Net.Http.HttpClient();

        var task = System.Threading.Tasks.Task.Run(async () => {
            var res = await client.PostAsync(new System.UriBuilder(url).Uri, content);
            res.EnsureSuccessStatusCode();
            return await res.Content.ReadAsStringAsync();
        });
        yield return new WaitUntil(() => task.IsCompleted);
        if (task.Exception != null) {
            Debug.LogError(task.Exception);
            yield break;
        }

        yield return setRemoteDescription(task.Result);
    }

    private IEnumerator setRemoteDescription(string answer)
    {
        RTCSessionDescription desc = new RTCSessionDescription();
        desc.type = RTCSdpType.Answer;
        desc.sdp = answer;
        var op = pc.SetRemoteDescription(ref desc);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetRemoteDescription() failed");
            yield break;
        }

        yield break;
    }

    void OnDestroy()
    {
        pc?.Close();
        pc?.Dispose();
        videoStream?.Dispose();
    }
}
```

In the _Hierarchy_ window, find or create a scene and a camera, then add the `WebRTCPublisher.cs` script as component of the camera, by dragging it inside the _Inspector_ window. then Press the _Play_ button at the top of the page.

The resulting stream will be available in path `/unity`.

#### Web browsers

Web browsers can publish a stream to the server by using the [WebRTC protocol](#webrtc). Start the server and open the web page:

```
http://localhost:8889/mystream/publish
```

The resulting stream will be available in path `/mystream`.

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="http://mediamtx-ip:8889/mystream/publish" scrolling="no"></iframe>
```

For more advanced setups, you can create and serve a custom web page by starting from the [source code of the WebRTC publish page](internal/servers/webrtc/publish_index.html).

### By device

#### Generic webcam

If the operating system is Linux-based, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f v4l2 -i /dev/video0 -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnInitRestart: yes
```

If the operating system is Windows:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f dshow -i video="USB2.0 HD UVC WebCam" -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnInitRestart: yes
```

Where `USB2.0 HD UVC WebCam` is the name of a webcam, that can be obtained with:

```sh
ffmpeg -list_devices true -f dshow -i dummy
```

The resulting stream will be available in path `/cam`.

#### Raspberry Pi Cameras

_MediaMTX_ natively supports most of the Raspberry Pi Camera models, enabling high-quality and low-latency video streaming from the camera to any user, for any purpose. There are a couple of requirements:

1. The server must run on a Raspberry Pi, with one of the following operating systems:

   * Raspberry Pi OS Bookworm
   * Raspberry Pi OS Bullseye

   Both 32 bit and 64 bit architectures are supported.

2. If you are using Raspberry Pi OS Bullseye, make sure that the legacy camera stack is disabled. Type `sudo raspi-config`, then go to `Interfacing options`, `enable/disable legacy camera support`, choose `no`. Reboot the system.

If you want to run the standard (non-Docker) version of the server:

1. Download the server executable. If you're using 64-bit version of the operative system, make sure to pick the `arm64` variant.

2. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

   ```yml
   paths:
     cam:
       source: rpiCamera
   ```

The resulting stream will be available in path `/cam`.

If you want to run the server inside Docker, you need to use the `latest-rpi` image and launch the container with some additional flags:

```sh
docker run --rm -it \
--network=host \
--privileged \
--tmpfs /dev/shm:exec \
-v /run/udev:/run/udev:ro \
-e MTX_PATHS_CAM_SOURCE=rpiCamera \
bluenviron/mediamtx:latest-rpi
```

Be aware that the server is not compatible with cameras that requires a custom `libcamera` (like some ArduCam products), since it comes with a bundled `libcamera`. If you want to use a custom one, you can [compile from source](#custom-libcamera).

Camera settings can be changed by using the `rpiCamera*` parameters:

```yml
paths:
  cam:
    source: rpiCamera
    rpiCameraWidth: 1920
    rpiCameraHeight: 1080
```

All available parameters are listed in the [sample configuration file](/mediamtx.yml).

In order to add audio from a USB microfone, install GStreamer and alsa-utils:

```sh
sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp gstreamer1.0-alsa alsa-utils
```

list available audio cards with:

```sh
arecord -L
```

Sample output:

```
surround51:CARD=ICH5,DEV=0
    Intel ICH5, Intel ICH5
    5.1 Surround output to Front, Center, Rear and Subwoofer speakers
default:CARD=U0x46d0x809
    USB Device 0x46d:0x809, USB Audio
    Default Audio Device
```

Find the audio card of the microfone and take note of its name, for instance `default:CARD=U0x46d0x809`. Then create a new path that takes the video stream from the camera and audio from the microphone:

```yml
paths:
  cam:
    source: rpiCamera

  cam_with_audio:
    runOnInit: >
      gst-launch-1.0
      rtspclientsink name=s location=rtsp://localhost:$RTSP_PORT/cam_with_audio
      rtspsrc location=rtsp://127.0.0.1:$RTSP_PORT/cam latency=0 ! rtph264depay ! s.
      alsasrc device=default:CARD=U0x46d0x809 ! opusenc bitrate=16000 ! s.
    runOnInitRestart: yes
```

The resulting stream will be available in path `/cam_with_audio`.

### By protocol

#### SRT clients

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to publish a stream to the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=publish:mystream&pkt_size=1316
```

Replace `mystream` with any name you want. The resulting stream will be available in path `/mystream`.

If credentials are enabled, append username and password to `streamid`:

```
srt://localhost:8890?streamid=publish:mystream:user:pass&pkt_size=1316
```

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](#standard-stream-id-syntax).

If you want to publish a stream by using a client in listening mode (i.e. with `mode=listener` appended to the URL), read the next section.

Known clients that can publish with SRT are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

#### SRT cameras and servers

In order to ingest into the server a SRT stream from an existing server, camera or client in listening mode (i.e. with `mode=listener` appended to the URL), add the corresponding URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # url of the source stream, in the format srt://host:port?streamid=streamid&other_parameters
    source: srt://original-url
```

#### WebRTC clients

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange real-time media or data streams. You can publish a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream/publish
```

The resulting stream will be available in path `/mystream`.

WHIP is a WebRTC extensions that allows to publish streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHIP (for instance, latest versions of OBS Studio), you can publish a stream to the server by using this URL:

```
http://localhost:8889/mystream/whip
```

Regarding authentication, read [Authenticating with WHIP/WHEP](#authenticating-with-whipwhep).

Depending on the network it may be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](#solving-webrtc-connectivity-issues).

Known clients that can publish with WebRTC and WHIP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio), [Unity](#unity) and [Web browsers](#web-browsers).

#### WebRTC servers

In order to ingest into the server a WebRTC stream from an existing server, add the corresponding WHEP URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # url of the source stream, in the format whep://host:port/path (HTTP) or wheps:// (HTTPS)
    source: wheps://host:port/path
```

#### RTSP clients

RTSP is a protocol that allows to publish and read streams. It supports different underlying transport protocols and allows to encrypt streams in transit (see [RTSP-specific features](#rtsp-specific-features)). In order to publish a stream to the server with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

The resulting stream will be available in path `/mystream`.

Known clients that can publish with RTSP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

#### RTSP cameras and servers

Most IP cameras expose their video stream by using a RTSP server that is embedded into the camera itself. In particular, cameras that are compliant with ONVIF profile S or T meet this requirement. You can use _MediaMTX_ to connect to one or multiple existing RTSP servers and read their video streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

The resulting stream will be available in path `/proxied`.

The server supports any number of source streams (count is just limited by available hardware resources) it's enough to add additional entries to the paths section:

```yml
paths:
  proxied1:
    source: rtsp://url1

  proxied2:
    source: rtsp://url1
```

#### RTMP clients

RTMP is a protocol that allows to read and publish streams, but is less versatile and less efficient than RTSP and WebRTC (doesn't support UDP, doesn't support most RTSP codecs, doesn't support feedback mechanism). Streams can be published to the server by using the URL:

```
rtmp://localhost/mystream
```

The resulting stream will be available in path `/mystream`.

In case authentication is enabled, credentials can be passed to the server by using the `user` and `pass` query parameters:

```
rtmp://localhost/mystream?user=myuser&pass=mypass
```

Known clients that can publish with RTMP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

#### RTMP cameras and servers

You can use _MediaMTX_ to connect to one or multiple existing RTMP servers and read their video streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtmp://user:pass@host:port/path
    source: rtmp://original-url
```

The resulting stream will be available in path `/proxied`.

#### HLS cameras and servers

HLS is a streaming protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to connect to one or multiple existing HLS servers and read their video streams:

```yml
paths:
  proxied:
    # url of the playlist of the stream, in the format http://user:pass@host:port/path
    source: http://original-url/stream/index.m3u8
```

The resulting stream will be available in path `/proxied`.

#### UDP/MPEG-TS

The server supports ingesting UDP/MPEG-TS packets (i.e. MPEG-TS packets sent with UDP). Packets can be unicast, broadcast or multicast. For instance, you can generate a multicast UDP/MPEG-TS stream with GStreamer:

```sh
gst-launch-1.0 -v mpegtsmux name=mux alignment=1 ! udpsink host=238.0.0.1 port=1234 \
videotestsrc ! video/x-raw,width=1280,height=720,format=I420 ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60 ! video/x-h264,profile=high ! mux. \
audiotestsrc ! audioconvert ! avenc_aac ! mux.
```

or FFmpeg:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f mpegts udp://238.0.0.1:1234?pkt_size=1316
```

Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  mypath:
    source: udp://238.0.0.1:1234
```

The resulting stream will be available in path `/mypath`.

Known clients that can publish with WebRTC and WHIP are [FFmpeg](#ffmpeg) and [GStreamer](#gstreamer).

## Read from the server

### By software

#### FFmpeg

FFmpeg can read a stream from the server in multiple ways (RTSP, RTMP, HLS, WebRTC with WHEP, SRT). The recommended one consists in reading with [RTSP](#rtsp):

```sh
ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
```

The RTSP protocol supports multiple underlying transport protocols, each with its own characteristics (see [RTSP-specific features](#rtsp-specific-features)). You can set the transport protocol by using the `rtsp_transport` flag:

```sh
ffmpeg -rtsp_transport tcp -i rtsp://localhost:8554/mystream -c copy output.mp4
```

#### GStreamer

GStreamer can read a stream from the server in multiple ways (RTSP, RTMP, HLS, WebRTC with WHEP, SRT). The recommended one consists in reading with [RTSP](#rtsp):

```sh
gst-launch-1.0 rtspsrc location=rtsp://127.0.0.1:8554/mystream latency=0 ! decodebin ! autovideosink
```

The RTSP protocol supports multiple underlying transport protocols, each with its own characteristics (see [RTSP-specific features](#rtsp-specific-features)). You can change the transport protocol by using the `protocols` flag:

```sh
gst-launch-1.0 rtspsrc protocols=tcp location=rtsp://127.0.0.1:8554/mystream latency=0 ! decodebin ! autovideosink
```

If encryption is enabled, set `tls-validation-flags` to `0`:

```sh
gst-launch-1.0 rtspsrc tls-validation-flags=0 location=rtsps://ip:8322/...
```

GStreamer also supports reading streams with WebRTC/WHEP, although track codecs must be specified in advance through the `video-caps` and `audio-caps` parameters. Furthermore, if audio is not present, `audio-caps` must be set anyway and must point to a PCMU codec. For instance, the command for reading a video-only H264 stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=PCMU,payload=0,clock-rate=8000" \
! rtph264depay ! decodebin ! autovideosink
```

While the command for reading an audio-only Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint="http://127.0.0.1:8889/stream/whep" use-link-headers=true \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! rtpopusdepay ! decodebin ! autoaudiosink
```

While the command for reading a H264 and Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! decodebin ! autovideosink
```

#### VLC

VLC can read a stream from the server in multiple ways (RTSP, RTMP, HLS, SRT). The recommended one consists in reading with [RTSP](#rtsp):

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream
```

The RTSP protocol supports multiple underlying transport protocols, each with its own characteristics (see [RTSP-specific features](#rtsp-specific-features)).

In order to use the TCP transport protocol, use the `--rtsp_tcp` flag:

```sh
vlc --network-caching=50 --rtsp-tcp rtsp://localhost:8554/mystream
```

In order to use the UDP-multicast transport protocol, append `?vlcmulticast` to the URL:

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream?vlcmulticast
```

##### Ubuntu bug

The VLC shipped with Ubuntu 21.10 doesn't support playing RTSP due to a license issue (see [here](https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=982299) and [here](https://stackoverflow.com/questions/69766748/cvlc-cannot-play-rtsp-omxplayer-instead-can)). To fix the issue, remove the default VLC instance and install the snap version:

```
sudo apt purge -y vlc
snap install vlc
```

##### Encrypted streams

At the moment VLC doesn't support reading encrypted RTSP streams. However, you can use a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/) or a local _MediaMTX_ instance to decrypt streams before reading them.

#### Unity

Software written with the Unity Engine can read a stream from the server by using the [WebRTC protocol](#webrtc).

Create a new Unity project or open an existing open.

Open _Window -> Package Manager_, click on the plus sign, _Add Package by name..._ and insert `com.unity.webrtc`. Wait for the package to be installed.

In the _Project_ window, under `Assets`, create a new C# Script called `WebRTCReader.cs` with this content:

```cs
using System.Collections;
using UnityEngine;
using Unity.WebRTC;

public class WebRTCReader : MonoBehaviour
{
    public string url = "http://localhost:8889/stream/whep";

    private RTCPeerConnection pc;
    private MediaStream receiveStream;

    void Start()
    {
        UnityEngine.UI.RawImage rawImage = gameObject.GetComponentInChildren<UnityEngine.UI.RawImage>();
        AudioSource audioSource = gameObject.GetComponentInChildren<AudioSource>();
        pc = new RTCPeerConnection();
        receiveStream = new MediaStream();

        pc.OnTrack = e =>
        {
            receiveStream.AddTrack(e.Track);
        };

        receiveStream.OnAddTrack = e =>
        {
            if (e.Track is VideoStreamTrack videoTrack)
            {
                videoTrack.OnVideoReceived += (tex) =>
                {
                    rawImage.texture = tex;
                };
            }
            else if (e.Track is AudioStreamTrack audioTrack)
            {
                audioSource.SetTrack(audioTrack);
                audioSource.loop = true;
                audioSource.Play();
            }
        };

        RTCRtpTransceiverInit init = new RTCRtpTransceiverInit();
        init.direction = RTCRtpTransceiverDirection.RecvOnly;
        pc.AddTransceiver(TrackKind.Audio, init);
        pc.AddTransceiver(TrackKind.Video, init);

        StartCoroutine(WebRTC.Update());
        StartCoroutine(createOffer());
    }

    private IEnumerator createOffer()
    {
        var op = pc.CreateOffer();
        yield return op;
        if (op.IsError) {
            Debug.LogError("CreateOffer() failed");
            yield break;
        }

        yield return setLocalDescription(op.Desc);
    }

    private IEnumerator setLocalDescription(RTCSessionDescription offer)
    {
        var op = pc.SetLocalDescription(ref offer);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetLocalDescription() failed");
            yield break;
        }

        yield return postOffer(offer);
    }

    private IEnumerator postOffer(RTCSessionDescription offer)
    {
        var content = new System.Net.Http.StringContent(offer.sdp);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/sdp");
        var client = new System.Net.Http.HttpClient();

        var task = System.Threading.Tasks.Task.Run(async () => {
            var res = await client.PostAsync(new System.UriBuilder(url).Uri, content);
            res.EnsureSuccessStatusCode();
            return await res.Content.ReadAsStringAsync();
        });
        yield return new WaitUntil(() => task.IsCompleted);
        if (task.Exception != null) {
            Debug.LogError(task.Exception);
            yield break;
        }

        yield return setRemoteDescription(task.Result);
    }

    private IEnumerator setRemoteDescription(string answer)
    {
        RTCSessionDescription desc = new RTCSessionDescription();
        desc.type = RTCSdpType.Answer;
        desc.sdp = answer;
        var op = pc.SetRemoteDescription(ref desc);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetRemoteDescription() failed");
            yield break;
        }

        yield break;
    }

    void OnDestroy()
    {
        pc?.Close();
        pc?.Dispose();
        receiveStream?.Dispose();
    }
}
```

Edit the `url` variable according to your needs.

In the _Hierarchy_ window, find or create a scene. Inside the scene, add a _Canvas_. Inside the Canvas, add a _Raw Image_ and an _Audio Source_. Then add the `WebRTCReader.cs` script as component of the canvas, by dragging it inside the _Inspector_ window. then Press the _Play_ button at the top of the page.

#### Web browsers

Web browsers can read a stream from the server in multiple ways (WebRTC or HLS).

You can read a stream by using the [WebRTC protocol](#webrtc-1) by visiting the web page:

```
http://localhost:8889/mystream
```

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="http://mediamtx-ip:8889/mystream" scrolling="no"></iframe>
```

For more advanced setups, you can create and serve a custom web page by starting from the [source code of the WebRTC read page](internal/servers/webrtc/read_index.html).

Web browsers can also read a stream with the [HLS protocol](#hls). Latency is higher but there are less problems related to connectivity between server and clients, furthermore the server load can be balanced by using a common HTTP CDN (like CloudFront or Cloudflare), and this allows to handle readers in the order of millions. Visit the web page:

```
http://localhost:8888/mystream
```

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="http://mediamtx-ip:8888/mystream" scrolling="no"></iframe>
```

For more advanced setups, you can create and serve a custom web page by starting from the [source code of the HLS read page](internal/servers/hls/index.html).

### By protocol

#### SRT

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to read a stream from the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=read:mystream
```

Replace `mystream` with the path name.

If credentials are enabled, append username and password to `streamid`:

```
srt://localhost:8890?streamid=read:mystream:user:pass
```

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](#standard-stream-id-syntax).

Known clients that can read with SRT are [FFmpeg](#ffmpeg-1), [GStreamer](#gstreamer-1) and [VLC](#vlc).

#### WebRTC

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange real-time media or data streams. You can read a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream
```

WHEP is a WebRTC extensions that allows to read streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHEP, you can read a stream from the server by using this URL:

```
http://localhost:8889/mystream/whep
```

Regarding authentication, read [Authenticating with WHIP/WHEP](#authenticating-with-whipwhep).

Depending on the network it may be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](#solving-webrtc-connectivity-issues).

Known clients that can read with WebRTC and WHEP are [FFmpeg](#ffmpeg-1), [GStreamer](#gstreamer-1), [Unity](#unity-1) and [web browsers](#web-browsers-1).

#### RTSP

RTSP is a protocol that allows to publish and read streams. It supports different underlying transport protocols and allows to encrypt streams in transit (see [RTSP-specific features](#rtsp-specific-features)). In order to read a stream with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

Known clients that can read with RTSP are [FFmpeg](#ffmpeg-1), [GStreamer](#gstreamer-1) and [VLC](#vlc).

##### Latency

The RTSP protocol doesn't introduce any latency by itself. Latency is usually introduced by clients, that put frames in a buffer to compensate network fluctuations. In order to decrease latency, the best way consists in tuning the client. For instance, in VLC, latency can be decreased by decreasing the _Network caching_ parameter, that is available in the _Open network stream_ dialog or alternatively can be set with the command line:

```
vlc --network-caching=50 rtsp://...
```

#### RTMP

RTMP is a protocol that allows to read and publish streams, but is less versatile and less efficient than RTSP and WebRTC (doesn't support UDP, doesn't support most RTSP codecs, doesn't support feedback mechanism). Streams can be read from the server by using the URL:

```
rtmp://localhost/mystream
```

In case authentication is enabled, credentials can be passed to the server by using the `user` and `pass` query parameters:

```
rtmp://localhost/mystream?user=myuser&pass=mypass
```

Known clients that can read with RTMP are [FFmpeg](#ffmpeg-1), [GStreamer](#gstreamer-1) and [VLC](#vlc).

#### HLS

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate a HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Known clients that can read with HLS are [FFmpeg](#ffmpeg-1), [GStreamer](#gstreamer-1), [VLC](#vlc) and [web browsers](#web-browsers-1).

##### LL-HLS

Low-Latency HLS is a recently standardized variant of the protocol that allows to greatly reduce playback latency. It works by splitting segments into parts, that are served before the segment is complete. LL-HLS is enabled by default. If the stream is not shown correctly, try tuning the hlsPartDuration parameter, for instance:

```yml
hlsPartDuration: 500ms
```

##### Compatibility with Apple devices

In order to correctly display Low-Latency HLS streams in Safari running on Apple devices (iOS or macOS), a TLS certificate is needed and can be generated with OpenSSL:

```sh
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Set the `hlsEncryption`, `hlsServerKey` and `hlsServerCert` parameters in the configuration file:

```yml
hlsEncryption: yes
hlsServerKey: server.key
hlsServerCert: server.crt
```

Keep also in mind that not all H264 video streams can be played on Apple Devices due to some intrinsic properties (distance between I-Frames, profile). If the video can't be played correctly, you can either:

* re-encode it by following instructions in this README
* disable the Low-latency variant of HLS and go back to the legacy variant:

  ```yml
    hlsVariant: mpegts
  ```

##### Latency

in HLS, latency is introduced since a client must wait for the server to generate segments before downloading them. This latency amounts to 500ms-3s when the low-latency HLS variant is enabled (and it is by default), otherwise amounts to 1-15secs.

To decrease the latency, you can:

* try decreasing the hlsPartDuration parameter
* try decreasing the hlsSegmentDuration parameter
* The segment duration is influenced by the interval between the IDR frames of the video track. An IDR frame is a frame that can be decoded independently from the others. The server changes the segment duration in order to include at least one IDR frame into each segment. Therefore, you need to decrease the interval between the IDR frames. This can be done in two ways:

  * if the stream is being hardware-generated (i.e. by a camera), there's usually a setting called Key-Frame Interval in the camera configuration page
  * otherwise, the stream must be re-encoded. It's possible to tune the IDR frame interval by using ffmpeg's -g option:

    ```sh
    ffmpeg -i rtsp://original-stream -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -max_muxing_queue_size 1024 -g 30 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    ```

## Other features

### Configuration

All the configuration parameters are listed and commented in the [configuration file](mediamtx.yml).

There are 3 ways to change the configuration:

1. By editing the `mediamtx.yml` file, that is

   * included into the release bundle
   * available in the root folder of the Docker image (`/mediamtx.yml`); it can be overridden in this way:

     ```
     docker run --rm -it --network=host -v "$PWD/mediamtx.yml:/mediamtx.yml:ro" bluenviron/mediamtx
     ```

   The configuration can be changed dynamically when the server is running (hot reloading) by writing to the configuration file. Changes are detected and applied without disconnecting existing clients, whenever it's possible.

2. By overriding configuration parameters with environment variables, in the format `MTX_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspAddress` parameter can be overridden in the following way:

   ```
   MTX_RTSPADDRESS="127.0.0.1:8554" ./mediamtx
   ```

   Parameters that have array as value can be overridden by setting a comma-separated list. For example:

   ```
   MTX_RTSPTRANSPORTS="tcp,udp"
   ```

   Parameters in maps can be overridden by using underscores, in the following way:

   ```
   MTX_PATHS_TEST_SOURCE=rtsp://myurl ./mediamtx
   ```

   This method is particularly useful when using Docker; any configuration parameter can be changed by passing environment variables with the `-e` flag:

   ```
   docker run --rm -it --network=host -e MTX_PATHS_TEST_SOURCE=rtsp://myurl bluenviron/mediamtx
   ```

3. By using the [Control API](#control-api).

### Authentication

#### Internal

The server provides three way to authenticate users:
* Internal: users are stored in the configuration file
* HTTP-based: an external HTTP URL is contacted to perform authentication
* JWT: an external identity server provides authentication through JWTs

The internal authentication method is the default one. Users are stored inside the configuration file, in this format:

```yml
authInternalUsers:
  # Username. 'any' means any user, including anonymous ones.
- user: any
  # Password. Not used in case of 'any' user.
  pass:
  # IPs or networks allowed to use this user. An empty list means any IP.
  ips: []
  # List of permissions.
  permissions:
    # Available actions are: publish, read, playback, api, metrics, pprof.
  - action: publish
    # Paths can be set to further restrict access to a specific path.
    # An empty path means any path.
    # Regular expressions can be used by using a tilde as prefix.
    path:
  - action: read
    path:
  - action: playback
    path:
```

Only clients that provide username and passwords will be able to perform a certain action:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://myuser:mypass@localhost:8554/mystream
```

If storing plain credentials in the configuration file is a security problem, username and passwords can be stored as hashed strings. The Argon2 and SHA256 hashing algorithms are supported. To use Argon2, the string must be hashed using Argon2id (recommended) or Argon2i:

```
echo -n "mypass" | argon2 saltItWithSalt -id -l 32 -e
```

Then stored with the `argon2:` prefix:

```yml
authInternalUsers:
- user: argon2:$argon2id$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$OGGO0eCMN0ievb4YGSzvS/H+Vajx1pcbUmtLp2tRqRU
  pass: argon2:$argon2i$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$oct3kOiFywTdDdt19kT07hdvmsPTvt9zxAUho2DLqZw
  permissions:
  - action: publish
```

To use SHA256, the string must be hashed with SHA256 and encoded with base64:

```
echo -n "mypass" | openssl dgst -binary -sha256 | openssl base64
```

Then stored with the `sha256:` prefix:

```yml
authInternalUsers:
- user: sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo=
  pass: sha256:BdSWkrdV+ZxFBLUQQY7+7uv9RmiSVA8nrPmjGjJtZQQ=
  permissions:
  - action: publish
```

**WARNING**: enable encryption or use a VPN to ensure that no one is intercepting the credentials in transit.

#### HTTP-based

Authentication can be delegated to an external HTTP server:

```yml
authMethod: http
authHTTPAddress: http://myauthserver/auth
```

Each time a user needs to be authenticated, the specified URL will be requested with the POST method and this payload:

```json
{
  "user": "user",
  "password": "password",
  "ip": "ip",
  "action": "publish|read|playback|api|metrics|pprof",
  "path": "path",
  "protocol": "rtsp|rtmp|hls|webrtc|srt",
  "id": "id",
  "query": "query"
}
```

If the URL returns a status code that begins with `20` (i.e. `200`), authentication is successful, otherwise it fails. Be aware that it's perfectly normal for the authentication server to receive requests with empty users and passwords, i.e.:

```json
{
  "user": "",
  "password": ""
}
```

This happens because RTSP clients don't provide credentials until they are asked to. In order to receive the credentials, the authentication server must reply with status code `401`, then the client will send credentials.

Some actions can be excluded from the process:

```yml
# Actions to exclude from HTTP-based authentication.
# Format is the same as the one of user permissions.
authHTTPExclude:
- action: api
- action: metrics
- action: pprof
```

#### JWT-based

Authentication can be delegated to an external identity server, that is capable of generating JWTs and provides a JWKS endpoint. With respect to the HTTP-based method, this has the advantage that the external server is contacted just once, and not for every request, greatly improving performance. In order to use the JWT-based authentication method, set `authMethod` and `authJWTJWKS`:

```yml
authMethod: jwt
authJWTJWKS: http://my_identity_server/jwks_endpoint
authJWTClaimKey: mediamtx_permissions
```

The JWT is expected to contain a claim, with a list of permissions in the same format as the one of user permissions:

```json
{
 "mediamtx_permissions": [
    {
      "action": "publish",
      "path": ""
    }
  ]
}
```

Clients are expected to pass the JWT in the Authorization header (in case of HLS, WebRTC and all web-based features) or in query parameters (in case of all other protocols), for instance:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream?jwt=MY_JWT
```

For instance (HLS):

```
GET /mypath/index.m3u8 HTTP/1.1
Host: example.com
Authorization: Bearer MY_JWT
```

Here's a tutorial on how to setup the [Keycloak identity server](https://www.keycloak.org/) in order to provide such JWTs:

1. Start Keycloak:

   ```
   docker run --name=keycloak -p 8080:8080 -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin quay.io/keycloak/keycloak:23.0.7 start-dev
   ```

2. Open the Keycloak administration console on http://localhost:8080, click on _master_ in the top left corner, _create realm_, set realm name to `mediamtx`, Save

3. Open page _Client scopes_, _create client scope_, set name to `mediamtx`, Save

4. Open tab _Mappers_, _Configure a new Mapper_, _User Attribute_

   * Name: `mediamtx_permissions`
   * User Attribute: `mediamtx_permissions`
   * Token Claim Name: `mediamtx_permissions`
   * Claim JSON Type: `JSON`
   * Multivalued: `On`

   Save

5. Open page _Clients_, _Create client_, set Client ID to `mediamtx`, Next, Client authentication `On`, Next, Save

6. Open tab _Credentials_, copy client secret somewhere

7. Open tab _Client scopes_, _Add client scope_, Select `mediamtx`, Add, Default

8. Open page _Users_, _Add user_, Username `testuser`, Tab credentials, _Set password_, pick a password, Save

9. Open tab _Attributes_, _Add an attribute_

   * Key: `mediamtx_permissions`
   * Value: `{"action":"publish", "path": ""}`

   You can add as many attributes with key `mediamtx_permissions` as you want, each with a single permission in it

10. In MediaMTX, use the following URL:

    ```yml
    authJWTJWKS: http://localhost:8080/realms/mediamtx/protocol/openid-connect/certs
    ```

11. Perform authentication on Keycloak:

    ```
    curl \
    -d "client_id=mediamtx" \
    -d "client_secret=$CLIENT_SECRET" \
    -d "username=$USER" \
    -d "password=$PASS" \
    -d "grant_type=password" \
    http://localhost:8080/realms/mediamtx/protocol/openid-connect/token
    ```

    The JWT is inside the `access_token` key of the response:

    ```json
    {"access_token":"eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICIyNzVjX3ptOVlOdHQ0TkhwWVk4Und6ZndUclVGSzRBRmQwY3lsM2wtY3pzIn0.eyJleHAiOjE3MDk1NTUwOTIsImlhdCI6MTcwOTU1NDc5MiwianRpIjoiMzE3ZTQ1NGUtNzczMi00OTM1LWExNzAtOTNhYzQ2ODhhYWIxIiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL3JlYWxtcy9tZWRpYW10eCIsImF1ZCI6ImFjY291bnQiLCJzdWIiOiI2NTBhZDA5Zi03MDgxLTQyNGItODI4Ni0xM2I3YTA3ZDI0MWEiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJtZWRpYW10eCIsInNlc3Npb25fc3RhdGUiOiJjYzJkNDhjYy1kMmU5LTQ0YjAtODkzZS0wYTdhNjJiZDI1YmQiLCJhY3IiOiIxIiwiYWxsb3dlZC1vcmlnaW5zIjpbIi8qIl0sInJlYWxtX2FjY2VzcyI6eyJyb2xlcyI6WyJvZmZsaW5lX2FjY2VzcyIsInVtYV9hdXRob3JpemF0aW9uIiwiZGVmYXVsdC1yb2xlcy1tZWRpYW10eCJdfSwicmVzb3VyY2VfYWNjZXNzIjp7ImFjY291bnQiOnsicm9sZXMiOlsibWFuYWdlLWFjY291bnQiLCJtYW5hZ2UtYWNjb3VudC1saW5rcyIsInZpZXctcHJvZmlsZSJdfX0sInNjb3BlIjoibWVkaWFtdHggcHJvZmlsZSBlbWFpbCIsInNpZCI6ImNjMmQ0OGNjLWQyZTktNDRiMC04OTNlLTBhN2E2MmJkMjViZCIsImVtYWlsX3ZlcmlmaWVkIjpmYWxzZSwibWVkaWFtdHhfcGVybWlzc2lvbnMiOlt7ImFjdGlvbiI6InB1Ymxpc2giLCJwYXRocyI6ImFsbCJ9XSwicHJlZmVycmVkX3VzZXJuYW1lIjoidGVzdHVzZXIifQ.Gevz7rf1qHqFg7cqtSfSP31v_NS0VH7MYfwAdra1t6Yt5rTr9vJzqUeGfjYLQWR3fr4XC58DrPOhNnILCpo7jWRdimCnbPmuuCJ0AYM-Aoi3PAsWZNxgmtopq24_JokbFArY9Y1wSGFvF8puU64lt1jyOOyxf2M4cBHCs_EarCKOwuQmEZxSf8Z-QV9nlfkoTUszDCQTiKyeIkLRHL2Iy7Fw7_T3UI7sxJjVIt0c6HCNJhBBazGsYzmcSQ_GrmhbUteMTg00o6FicqkMBe99uZFnx9wIBm_QbO9hbAkkzF923I-DTAQrFLxT08ESMepDwmzFrmnwWYBLE3u8zuUlCA","expires_in":300,"refresh_expires_in":1800,"refresh_token":"eyJhbGciOiJIUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICI3OTI3Zjg4Zi05YWM4LTRlNmEtYWE1OC1kZmY0MDQzZDRhNGUifQ.eyJleHAiOjE3MDk1NTY1OTIsImlhdCI6MTcwOTU1NDc5MiwianRpIjoiMGVhZWFhMWItYzNhMC00M2YxLWJkZjAtZjI2NTRiODlkOTE3IiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL3JlYWxtcy9tZWRpYW10eCIsImF1ZCI6Imh0dHA6Ly9sb2NhbGhvc3Q6ODA4MC9yZWFsbXMvbWVkaWFtdHgiLCJzdWIiOiI2NTBhZDA5Zi03MDgxLTQyNGItODI4Ni0xM2I3YTA3ZDI0MWEiLCJ0eXAiOiJSZWZyZXNoIiwiYXpwIjoibWVkaWFtdHgiLCJzZXNzaW9uX3N0YXRlIjoiY2MyZDQ4Y2MtZDJlOS00NGIwLTg5M2UtMGE3YTYyYmQyNWJkIiwic2NvcGUiOiJtZWRpYW10eCBwcm9maWxlIGVtYWlsIiwic2lkIjoiY2MyZDQ4Y2MtZDJlOS00NGIwLTg5M2UtMGE3YTYyYmQyNWJkIn0.yuXV8_JU0TQLuosNdp5xlYMjn7eO5Xq-PusdHzE7bsQ","token_type":"Bearer","not-before-policy":0,"session_state":"cc2d48cc-d2e9-44b0-893e-0a7a62bd25bd","scope":"mediamtx profile email"}
    ```

### Encrypt the configuration

The configuration file can be entirely encrypted for security purposes by using the `crypto_secretbox` function of the NaCL function. An online tool for performing this operation is [available here](https://play.golang.org/p/rX29jwObNe4).

After performing the encryption, put the base64-encoded result into the configuration file, and launch the server with the `MTX_CONFKEY` variable:

```
MTX_CONFKEY=mykey ./mediamtx
```

### Remuxing, re-encoding, compression

To change the format, codec or compression of a stream, use _FFmpeg_ or _GStreamer_ together with _MediaMTX_. For instance, to re-encode an existing stream, that is available in the `/original` path, and publish the resulting stream in the `/compressed` path, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  compressed:
  original:
    runOnReady: >
      ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
        -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k
        -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    runOnReadyRestart: yes
```

### Record streams to disk

To save available streams to disk, set the `record` and the `recordPath` parameter in the configuration file:

```yml
pathDefaults:
  # Record streams to disk.
  record: yes
  # Path of recording segments.
  # Extension is added automatically.
  # Available variables are %path (path name), %Y %m %d %H %M %S %f %s (time in strftime format)
  recordPath: ./recordings/%path/%Y-%m-%d_%H-%M-%S-%f
```

All available recording parameters are listed in the [sample configuration file](/mediamtx.yml).

Be aware that not all codecs can be saved with all formats, as described in the compatibility matrix at the beginning of the README.

To upload recordings to a remote location, you can use _MediaMTX_ together with [rclone](https://github.com/rclone/rclone), a command line tool that provides file synchronization capabilities with a huge variety of services (including S3, FTP, SMB, Google Drive):

1. Download and install [rclone](https://github.com/rclone/rclone).

2. Configure _rclone_:

   ```
   rclone config
   ```

3. Place `rclone` into the `runOnInit` and `runOnRecordSegmentComplete` hooks:

   ```yml
   pathDefaults:
     # this is needed to sync segments after a crash.
     # replace myconfig with the name of the rclone config.
     runOnInit: rclone sync -v ./recordings myconfig:/my-path/recordings

     # this is called when a segment has been finalized.
     # replace myconfig with the name of the rclone config.
     runOnRecordSegmentComplete: rclone sync -v --min-age=1ms ./recordings myconfig:/my-path/recordings
   ```

   If you want to delete local segments after they are uploaded, replace `rclone sync` with `rclone move`.

### Playback recorded streams

Existing recordings can be served to users through a dedicated HTTP server, that can be enabled inside the configuration:

```yml
playback: yes
playbackAddress: :9996
```

The server provides an endpoint to list recorded timespans:

```
http://localhost:9996/list?path=[mypath]&start=[start]&end=[end]
```

Where:

* [mypath] is the name of a path
* [start] (optional) is the start date in [RFC3339 format](https://www.utctime.net/)
* [end] (optional) is the end date in [RFC3339 format](https://www.utctime.net/)

The server will return a list of timespans in JSON format:

```json
[
  {
    "start": "2006-01-02T15:04:05Z07:00",
    "duration": "60.0",
    "url": "http://localhost:9996/get?path=[mypath]&start=2006-01-02T15%3A04%3A05Z07%3A00&duration=60.0"
  },
  {
    "start": "2006-01-02T15:07:05Z07:00",
    "duration": "32.33",
    "url": "http://localhost:9996/get?path=[mypath]&start=2006-01-02T15%3A07%3A05Z07%3A00&duration=32.33"
  }
]
```

The server provides an endpoint to download recordings:

```
http://localhost:9996/get?path=[mypath]&start=[start]&duration=[duration]&format=[format]
```

Where:

* [mypath] is the path name
* [start] is the start date in [RFC3339 format](https://www.utctime.net/)
* [duration] is the maximum duration of the recording in seconds
* [format] (optional) is the output format of the stream. Available values are "fmp4" (default) and "mp4"

All parameters must be [url-encoded](https://www.urlencoder.org/). For instance:

```
http://localhost:9996/get?path=mypath&start=2024-01-14T16%3A33%3A17%2B00%3A00&duration=200.5
```

The resulting stream uses the fMP4 format, that is natively compatible with any browser, therefore its URL can be directly inserted into a \<video> tag:

```html
<video controls>
  <source src="http://localhost:9996/get?path=[mypath]&start=[start_date]&duration=[duration]" type="video/mp4" />
</video>
```

The fMP4 format may offer limited compatibility with some players. To fix the issue, it's possible to use the standard MP4 format, by adding `format=mp4` to a `/get` request:

```
http://localhost:9996/get?path=[mypath]&start=[start_date]&duration=[duration]&format=mp4
```

### Forward streams to other servers

To forward incoming streams to another server, use _FFmpeg_ inside the `runOnReady` parameter:

```yml
pathDefaults:
  runOnReady: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnReadyRestart: yes
```

### Proxy requests to other servers

The server allows to proxy incoming requests to other servers or cameras. This is useful to expose servers or cameras behind a NAT. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  "~^proxy_(.+)$":
    # If path name is a regular expression, $G1, G2, etc will be replaced
    # with regular expression groups.
    source: rtsp://other-server:8554/$G1
    sourceOnDemand: yes
```

All requests addressed to `rtsp://server:8854/proxy_a` will be forwarded to `rtsp://other-server:8854/a` and so on.

### On-demand publishing

Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  ondemand:
    runOnDemand: ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnDemandRestart: yes
```

The command inserted into `runOnDemand` will start only when a client requests the path `ondemand`, therefore the file will start streaming only when requested.

### Start on boot

#### Linux

On most Linux distributions (including Ubuntu and Debian, but not OpenWrt), _systemd_ is in charge of managing services and starting them on boot.

Move the server executable and configuration in global folders:

```sh
sudo mv mediamtx /usr/local/bin/
sudo mv mediamtx.yml /usr/local/etc/
```

Create a _systemd_ service:

```sh
sudo tee /etc/systemd/system/mediamtx.service >/dev/null << EOF
[Unit]
Wants=network.target
[Service]
ExecStart=/usr/local/bin/mediamtx /usr/local/etc/mediamtx.yml
[Install]
WantedBy=multi-user.target
EOF
```

If SELinux is enabled (for instance in case of RedHat, Rocky, CentOS++), add correct security context:

```sh
semanage fcontext -a -t bin_t /usr/local/bin/mediamtx
restorecon -Fv /usr/local/bin/mediamtx
```

Enable and start the service:

```sh
sudo systemctl daemon-reload
sudo systemctl enable mediamtx
sudo systemctl start mediamtx
```

#### OpenWrt

Move the server executable and configuration in global folders:

```sh
mv mediamtx /usr/bin/
mkdir -p /usr/etc && mv mediamtx.yml /usr/etc/
```

Create a procd service:

```sh
tee /etc/init.d/mediamtx >/dev/null << EOF
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=95
STOP=01
start_service() {
    procd_open_instance
    procd_set_param command /usr/bin/mediamtx
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
```

Enable and start the service:

```sh
chmod +x /etc/init.d/mediamtx
/etc/init.d/mediamtx enable
/etc/init.d/mediamtx start
```

Read the server logs:

```sh
logread
```

#### Windows

Download the [WinSW v2 executable](https://github.com/winsw/winsw/releases/download/v2.11.0/WinSW-x64.exe) and place it into the same folder of `mediamtx.exe`.

In the same folder, create a file named `WinSW-x64.xml` with this content:

```xml
<service>
  <id>mediamtx</id>
  <name>mediamtx</name>
  <description></description>
  <executable>%BASE%/mediamtx.exe</executable>
</service>
```

Open a terminal, navigate to the folder and run:

```
WinSW-x64 install
```

The server is now installed as a system service and will start at boot time.

### Hooks

The server allows to specify commands that are executed when a certain event happens, allowing the propagation of events to external software.

`runOnConnect` allows to run a command when a client connects to the server:

```yml
# Command to run when a client connects to the server.
# This is terminated with SIGINT when a client disconnects from the server.
# The following environment variables are available:
# * MTX_CONN_TYPE: connection type
# * MTX_CONN_ID: connection ID
# * RTSP_PORT: RTSP server port
runOnConnect: curl http://my-custom-server/webhook?conn_type=$MTX_CONN_TYPE&conn_id=$MTX_CONN_ID
# Restart the command if it exits.
runOnConnectRestart: no
```

`runOnDisconnect` allows to run a command when a client disconnects from the server:

```yml
# Command to run when a client disconnects from the server.
# Environment variables are the same of runOnConnect.
runOnDisconnect: curl http://my-custom-server/webhook?conn_type=$MTX_CONN_TYPE&conn_id=$MTX_CONN_ID
```

`runOnInit` allows to run a command when a path is initialized. This can be used to publish a stream when the server is launched:

```yml
paths:
  mypath:
    # Command to run when this path is initialized.
    # This can be used to publish a stream when the server is launched.
    # The following environment variables are available:
    # * MTX_PATH: path name
    # * RTSP_PORT: RTSP server port
    # * G1, G2, ...: regular expression groups, if path name is
    #   a regular expression.
    runOnInit: ffmpeg -i my_file.mp4 -c copy -f rtsp rtsp://localhost:8554/mypath
    # Restart the command if it exits.
    runOnInitRestart: no
```

`runOnDemand` allows to run a command when a path is requested by a reader. This can be used to publish a stream on demand:

```yml
pathDefaults:
  # Command to run when this path is requested by a reader
  # and no one is publishing to this path yet.
  # This is terminated with SIGINT when there are no readers anymore.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by first reader)
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnDemand: ffmpeg -i my_file.mp4 -c copy -f rtsp rtsp://localhost:8554/mypath
  # Restart the command if it exits.
  runOnDemandRestart: no
```

`runOnUnDemand` allows to run a command when there are no readers anymore:

```yml
pathDefaults:
  # Command to run when there are no readers anymore.
  # Environment variables are the same of runOnDemand.
  runOnUnDemand:
```

`runOnReady` allows to run a command when a stream is ready to be read:

```yml
pathDefaults:
  # Command to run when the stream is ready to be read, whenever it is
  # published by a client or pulled from a server / camera.
  # This is terminated with SIGINT when the stream is not ready anymore.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by publisher)
  # * MTX_SOURCE_TYPE: source type
  # * MTX_SOURCE_ID: source ID
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnReady: curl http://my-custom-server/webhook?path=$MTX_PATH&source_type=$MTX_SOURCE_TYPE&source_id=$MTX_SOURCE_ID
  # Restart the command if it exits.
  runOnReadyRestart: no
```

`runOnNotReady` allows to run a command when a stream is not available anymore:

```yml
pathDefaults:
  # Command to run when the stream is not available anymore.
  # Environment variables are the same of runOnReady.
  runOnNotReady: curl http://my-custom-server/webhook?path=$MTX_PATH&source_type=$MTX_SOURCE_TYPE&source_id=$MTX_SOURCE_ID
```

`runOnRead` allows to run a command when a client starts reading:

```yml
pathDefaults:
  # Command to run when a client starts reading.
  # This is terminated with SIGINT when a client stops reading.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by reader)
  # * MTX_READER_TYPE: reader type
  # * MTX_READER_ID: reader ID
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRead: curl http://my-custom-server/webhook?path=$MTX_PATH&reader_type=$MTX_READER_TYPE&reader_id=$MTX_READER_ID
  # Restart the command if it exits.
  runOnReadRestart: no
```

`runOnUnread` allows to run a command when a client stops reading:

```yml
pathDefaults:
  # Command to run when a client stops reading.
  # Environment variables are the same of runOnRead.
  runOnUnread: curl http://my-custom-server/webhook?path=$MTX_PATH&reader_type=$MTX_READER_TYPE&reader_id=$MTX_READER_ID
```

`runOnRecordSegmentCreate` allows to run a command when a recording segment is created:

```yml
pathDefaults:
  # Command to run when a recording segment is created.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_SEGMENT_PATH: segment file path
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRecordSegmentCreate: curl http://my-custom-server/webhook?path=$MTX_PATH&segment_path=$MTX_SEGMENT_PATH
```

`runOnRecordSegmentComplete` allows to run a command when a recording segment is complete:

```yml
pathDefaults:
  # Command to run when a recording segment is complete.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_SEGMENT_PATH: segment file path
  # * MTX_SEGMENT_DURATION: segment duration
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRecordSegmentComplete: curl http://my-custom-server/webhook?path=$MTX_PATH&segment_path=$MTX_SEGMENT_PATH
```

### Control API

The server can be queried and controlled with an API, that can be enabled by setting the `api` parameter in the configuration:

```yml
api: yes
```

To obtain a list of of active paths, run:

```
curl http://127.0.0.1:9997/v3/paths/list
```

Full documentation of the Control API is available on the [dedicated site](https://bluenviron.github.io/mediamtx/).

Be aware that by default the Control API is accessible by localhost only; to increase visibility or add authentication, check [Authentication](#authentication).

### Metrics

A metrics exporter, compatible with [Prometheus](https://prometheus.io/), can be enabled with the parameter `metrics: yes`; then the server can be queried for metrics with Prometheus or with a simple HTTP request:

```
curl localhost:9998/metrics
```

Obtaining:

```ini
# metrics of every path
paths{name="[path_name]",state="[state]"} 1
paths_bytes_received{name="[path_name]",state="[state]"} 1234
paths_bytes_sent{name="[path_name]",state="[state]"} 1234

# metrics of every HLS muxer
hls_muxers{name="[name]"} 1
hls_muxers_bytes_sent{name="[name]"} 187

# metrics of every RTSP connection
rtsp_conns{id="[id]"} 1
rtsp_conns_bytes_received{id="[id]"} 1234
rtsp_conns_bytes_sent{id="[id]"} 187

# metrics of every RTSP session
rtsp_sessions{id="[id]",state="idle"} 1
rtsp_sessions_bytes_received{id="[id]",state="[state]"} 1234
rtsp_sessions_bytes_sent{id="[id]",state="[state]"} 187
rtsp_sessions_rtp_packets_received{id="[id]"} 123
rtsp_sessions_rtp_packets_sent{id="[id]"} 123
rtsp_sessions_rtp_packets_lost{id="[id]"} 123
rtsp_sessions_rtp_packets_in_error{id="[id]"} 123
rtsp_sessions_rtp_packets_jitter{id="[id]"} 123
rtsp_sessions_rtcp_packets_received{id="[id]"} 123
rtsp_sessions_rtcp_packets_sent{id="[id]"} 123
rtsp_sessions_rtcp_packets_in_error{id="[id]"} 123

# metrics of every RTSPS connection
rtsps_conns{id="[id]"} 1
rtsps_conns_bytes_received{id="[id]"} 1234
rtsps_conns_bytes_sent{id="[id]"} 187

# metrics of every RTSPS session
rtsps_sessions{id="[id]",state="[state]"} 1
rtsps_sessions_bytes_received{id="[id]",state="[state]"} 1234
rtsps_sessions_bytes_sent{id="[id]",state="[state]"} 187
rtsps_sessions_rtp_packets_received{id="[id]"} 123
rtsps_sessions_rtp_packets_sent{id="[id]"} 123
rtsps_sessions_rtp_packets_lost{id="[id]"} 123
rtsps_sessions_rtp_packets_in_error{id="[id]"} 123
rtsps_sessions_rtp_packets_jitter{id="[id]"} 123
rtsps_sessions_rtcp_packets_received{id="[id]"} 123
rtsps_sessions_rtcp_packets_sent{id="[id]"} 123
rtsps_sessions_rtcp_packets_in_error{id="[id]"} 123

# metrics of every RTMP connection
rtmp_conns{id="[id]",state="[state]"} 1
rtmp_conns_bytes_received{id="[id]",state="[state]"} 1234
rtmp_conns_bytes_sent{id="[id]",state="[state]"} 187

# metrics of every RTMPS connection
rtmps_conns{id="[id]",state="[state]"} 1
rtmps_conns_bytes_received{id="[id]",state="[state]"} 1234
rtmps_conns_bytes_sent{id="[id]",state="[state]"} 187

# metrics of every SRT connection
srt_conns{id="[id]",state="[state]"} 1
srt_conns_packets_sent{id="[id]",state="[state]"} 123
srt_conns_packets_received{id="[id]",state="[state]"} 123
srt_conns_packets_sent_unique{id="[id]",state="[state]"} 123
srt_conns_packets_received_unique{id="[id]",state="[state]"} 123
srt_conns_packets_send_loss{id="[id]",state="[state]"} 123
srt_conns_packets_received_loss{id="[id]",state="[state]"} 123
srt_conns_packets_retrans{id="[id]",state="[state]"} 123
srt_conns_packets_received_retrans{id="[id]",state="[state]"} 123
srt_conns_packets_sent_ack{id="[id]",state="[state]"} 123
srt_conns_packets_received_ack{id="[id]",state="[state]"} 123
srt_conns_packets_sent_nak{id="[id]",state="[state]"} 123
srt_conns_packets_received_nak{id="[id]",state="[state]"} 123
srt_conns_packets_sent_km{id="[id]",state="[state]"} 123
srt_conns_packets_received_km{id="[id]",state="[state]"} 123
srt_conns_us_snd_duration{id="[id]",state="[state]"} 123
srt_conns_packets_send_drop{id="[id]",state="[state]"} 123
srt_conns_packets_received_drop{id="[id]",state="[state]"} 123
srt_conns_packets_received_undecrypt{id="[id]",state="[state]"} 123
srt_conns_bytes_sent{id="[id]",state="[state]"} 187
srt_conns_bytes_received{id="[id]",state="[state]"} 1234
srt_conns_bytes_sent_unique{id="[id]",state="[state]"} 123
srt_conns_bytes_received_unique{id="[id]",state="[state]"} 123
srt_conns_bytes_received_loss{id="[id]",state="[state]"} 123
srt_conns_bytes_retrans{id="[id]",state="[state]"} 123
srt_conns_bytes_received_retrans{id="[id]",state="[state]"} 123
srt_conns_bytes_send_drop{id="[id]",state="[state]"} 123
srt_conns_bytes_received_drop{id="[id]",state="[state]"} 123
srt_conns_bytes_received_undecrypt{id="[id]",state="[state]"} 123
srt_conns_us_packets_send_period{id="[id]",state="[state]"} 123.123
srt_conns_packets_flow_window{id="[id]",state="[state]"} 123
srt_conns_packets_flight_size{id="[id]",state="[state]"} 123
srt_conns_ms_rtt{id="[id]",state="[state]"} 123.123
srt_conns_mbps_send_rate{id="[id]",state="[state]"} 123
srt_conns_mbps_receive_rate{id="[id]",state="[state]"} 123.123
srt_conns_mbps_link_capacity{id="[id]",state="[state]"} 123.123
srt_conns_bytes_avail_send_buf{id="[id]",state="[state]"} 123
srt_conns_bytes_avail_receive_buf{id="[id]",state="[state]"} 123
srt_conns_mbps_max_bw{id="[id]",state="[state]"} -123
srt_conns_bytes_mss{id="[id]",state="[state]"} 123
srt_conns_packets_send_buf{id="[id]",state="[state]"} 123
srt_conns_bytes_send_buf{id="[id]",state="[state]"} 123
srt_conns_ms_send_buf{id="[id]",state="[state]"} 123
srt_conns_ms_send_tsb_pd_delay{id="[id]",state="[state]"} 123
srt_conns_packets_receive_buf{id="[id]",state="[state]"} 123
srt_conns_bytes_receive_buf{id="[id]",state="[state]"} 123
srt_conns_ms_receive_buf{id="[id]",state="[state]"} 123
srt_conns_ms_receive_tsb_pd_delay{id="[id]",state="[state]"} 123
srt_conns_packets_reorder_tolerance{id="[id]",state="[state]"} 123
srt_conns_packets_received_avg_belated_time{id="[id]",state="[state]"} 123
srt_conns_packets_send_loss_rate{id="[id]",state="[state]"} 123
srt_conns_packets_received_loss_rate{id="[id]",state="[state]"} 123

# metrics of every WebRTC session
webrtc_sessions{id="[id]",state="[state]"} 1
webrtc_sessions_bytes_received{id="[id]",state="[state]"} 1234
webrtc_sessions_bytes_sent{id="[id]",state="[state]"} 187
```

### pprof

A performance monitor, compatible with pprof, can be enabled with the parameter `pprof: yes`; then the server can be queried for metrics with pprof-compatible tools, like:

```
go tool pprof -text http://localhost:9999/debug/pprof/goroutine
go tool pprof -text http://localhost:9999/debug/pprof/heap
go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=30
```

### SRT-specific features

#### Standard stream ID syntax

In SRT, the stream ID is a string that is sent to the counterpart in order to advertise what action the caller is gonna do (publish or read), the path and the credentials. All these informations have to be encoded into a single string. This server supports two stream ID syntaxes, a custom one (that is the one reported in rest of the README) and also a [standard one](https://github.com/Haivision/srt/blob/master/docs/features/access-control.md) proposed by the authors of the protocol and sometimes enforced by some hardware. The standard syntax can be used in this way:

```
srt://localhost:8890?streamid=#!::m=publish,r=mypath,u=myuser,s=mypass&pkt_size=1316
```

Where:

* key `m` contains the action (`publish` or `request`)
* key `r` contains the path
* key `u` contains the username
* key `s` contains the password

### WebRTC-specific features

#### Authenticating with WHIP/WHEP

When using WHIP or WHEP to establish a WebRTC connection, there are multiple ways to provide credentials.

If internal authentication or HTTP-based authentication is enabled, username and password can be passed through the `Authentication: Basic` header:

```
Authentication: Basic [base64_encoded_credentials]
```

Username and password can be also passed through the `Authentication: Bearer` header (since it's mandated by the specification):

```
Authentication: Bearer username:password
```

If JWT-based authentication is enabled, JWT can be passed through the `Authentication: Bearer` header:

```
Authentication: Bearer [jwt]
```

The JWT can also be passed through query parameters:

```
http://localhost:8889/mystream/whip?jwt=[jwt]
```

#### Solving WebRTC connectivity issues

If the server is hosted inside a container or is behind a NAT, additional configuration is required in order to allow the two WebRTC parts (server and client) to establish a connection.

Make sure that `webrtcAdditionalHosts` includes your public IPs, that are IPs that can be used by clients to reach the server. If clients are on the same LAN as the server, then insert the LAN address of the server. If clients are coming from the internet, insert the public IP address of the server, or alternatively a DNS name, if you have one. You can insert multiple values to support all scenarios:

```yml
webrtcAdditionalHosts: [192.168.x.x, 1.2.3.4, my-dns.example.org, ...]
```

If there's a NAT / container between server and clients, it must be configured to route all incoming UDP packets on port 8189 to the server. If you're using Docker, this can be achieved with the flag:

```sh
docker run --rm -it \
-p 8189:8189/udp
....
bluenviron/mediamtx
```

If you still have problems, the UDP protocol might be blocked by a firewall. Enable the TCP protocol by enabling the local TCP listener:

```yml
webrtcLocalTCPAddress: :8189
```

If there's a NAT / container between server and clients, it must be configured to route all incoming TCP packets on port 8189 to the server.

If you still have problems, enable a STUN server:

```yml
webrtcICEServers2:
  - url: stun:stun.l.google.com:19302
```

When a STUN server is in use, connections can be established with the "UDP hole punching" method, that uses a random UDP port that does not need to be open.

If you really still have problems, you can force all WebRTC/ICE connections to pass through a TURN server, like [coturn](https://github.com/coturn/coturn), that must be configured externally. The server address and credentials must be set in the configuration file:

```yml
webrtcICEServers2:
- url: turn:host:port
  username: user
  password: password
```

Where user and pass are the username and password of the server. Note that port is not optional.

If the server uses a secret-based authentication (for instance, coturn with the use-auth-secret option), it must be configured by using AUTH_SECRET as username, and the secret as password:

```yml
webrtcICEServers2:
- url: turn:host:port
  username: AUTH_SECRET
  password: secret
```

where secret is the secret of the TURN server. MediaMTX will generate a set of credentials by using the secret, and credentials will be sent to clients before the WebRTC/ICE connection is established.

In some cases you may want the browser to connect using TURN servers but have mediamtx not using TURN (for example if the TURN server is on the same network as mediamtx).  To allow this you can configure the TURN server to be client only:

```yml
webrtcICEServers2:
- url: turn:host:port
  username: user
  password: password
  clientOnly: true
```

#### Supported browsers

The server can ingest and broadcast with WebRTC a wide variety of video and audio codecs (that are listed at the beginning of the README), but not all browsers can publish and read all codecs due to internal limitations that cannot be overcome by this or any other server.

In particular, reading and publishing H265 tracks with WebRTC was not possible until some time ago due to the lack of browser support. The situation recently improved and can be described as following:

* Safari on iOS and macOS fully supports publishing and reading H265 tracks
* Chrome on Windows supports publishing and reading H265 tracks when a GPU is present and when the browser is launched with the following flags:

  ```
  chrome.exe --enable-features=PlatformHEVCEncoderSupport,WebRtcAllowH265Receive,WebRtcAllowH265Send --force-fieldtrials=WebRTC-Video-H26xPacketBuffer/Enabled
  ```

  We are expecting these flags to become redundant in the future and the feature to be turned on by default.

You can check what codecs your browser can publish or read with WebRTC by [using this tool](https://jsfiddle.net/v24s8q1f/).

If you want to support most browsers, you can to re-encode the stream by using H264 and Opus codecs, for instance by using FFmpeg:

```sh
ffmpeg -i rtsp://original-source \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a libopus -b:a 64K -async 50 \
-f rtsp rtsp://localhost:8554/mystream
```

### HLS-specific features

#### Supported browsers

The server can produce HLS streams with a variety of video and audio codecs (that are listed at the beginning of the README), but not all browsers can read all codecs due to internal limitations that cannot be overcome by this or any other server.

You can check what codecs your browser can read with HLS by [using this tool](https://jsfiddle.net/tjcyv5aw/).

If you want to support most browsers, you can to re-encode the stream by using H264 and AAC codecs, for instance by using FFmpeg:

```sh
ffmpeg -i rtsp://original-source \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a aac -b:a 160k \
-f rtsp rtsp://localhost:8554/mystream
```

### RTSP-specific features

#### Transport protocols

The RTSP protocol supports different underlying transport protocols, that are chosen by clients during the handshake with the server:

* UDP: the most performant, but doesn't work when there's a NAT/firewall between server and clients. It doesn't support encryption.
* UDP-multicast: allows to save bandwidth when clients are all in the same LAN, by sending packets once to a fixed multicast IP. It doesn't support encryption.
* TCP: the most versatile, does support encryption.

The default transport protocol is UDP. To change the transport protocol, you have to tune the configuration of your client of choice.

#### Encryption

Incoming and outgoing RTSP streams can be encrypted with TLS, obtaining the RTSPS protocol. A TLS certificate is needed and can be generated with OpenSSL:

```sh
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit `mediamtx.yml` and set the `rtspTransports`, `encryption`, `serverKey` and serverCert parameters:

```yml
rtspTransports: [tcp]
rtspEncryption: optional
rtspServerKey: server.key
rtspServerCert: server.crt
```

Streams can be published and read with the `rtsps` scheme and the `8322` port:

```
rtsps://localhost:8322/mystream
```

#### Corrupted frames

In some scenarios, when publishing or reading from the server with RTSP, frames can get corrupted. This can be caused by multiple reasons:

* the write queue of the server is too small and can't keep up with the stream throughput. A solution consists in increasing its size:

  ```yml
  writeQueueSize: 1024
  ```

* The stream throughput is too big and the stream can't be transmitted correctly with the UDP transport protocol. UDP is more performant, faster and more efficient than TCP, but doesn't have a retransmission mechanism, that is needed in case of streams that need a large bandwidth. A solution consists in switching to TCP:

  ```yml
  rtspTransports: [tcp]
  ```

  In case the source is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      rtspTransport: tcp
   ```

* The stream throughput is too big to be handled by the network between server and readers. Upgrade the network or decrease the stream bitrate by re-encoding it.

### RTMP-specific features

#### Encryption

RTMP connections can be encrypted with TLS, obtaining the RTMPS protocol. A TLS certificate is needed and can be generated with OpenSSL:

```yml
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit mediamtx.yml and set the `rtmpEncryption`, `rtmpServerKey` and `rtmpServerCert` parameters:

```yml
rtmpEncryption: optional
rtmpServerKey: server.key
rtmpServerCert: server.crt
```

Streams can be published and read with the rtmps scheme and the 1937 port:

```
rtmps://localhost:1937/...
```

Be aware that RTMPS is currently unsupported by all major players. However, you can use a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/) or a dedicated _MediaMTX_ instance to decrypt streams before reading them.

## Compile from source

### Standard

Install git and Go &ge; 1.23. Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 go build .
```

The command will produce the `mediamtx` binary.

### OpenWrt

The compilation procedure is the same as the standard one. On the OpenWrt device, install git and Go:

```sh
opkg update
opkg install golang git git-http
```

Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 go build .
```

The command will produce the `mediamtx` binary.

If the OpenWrt device doesn't have enough resources to compile, you can [cross compile](#cross-compile) from another machine.

### Custom libcamera

If you need to use a custom or external libcamera when interacting with the Raspberry Pi Camera, you have to compile [mediamtx-rpicamera](https://github.com/bluenviron/mediamtx-rpicamera) before compiling the server. Instructions are present in the `mediamtx-rpicamera` repository.

### Cross compile

Cross compilation allows to build an executable for a target machine from another machine with different operating system or architecture. This is useful in case the target machine doesn't have enough resources for compilation or if you don't want to install the compilation dependencies on it.

On the machine you want to use to compile, install git and Go &ge; 1.23. Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 GOOS=my_os GOARCH=my_arch go build .
```

Replace `my_os` and `my_arch` with the operating system and architecture of your target machine. A list of all supported combinations can be obtained with:

```sh
go tool dist list
```

For instance:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build .
```

In case of the `arm` architecture, there's an additional flag available, `GOARM`, that allows to set the ARM version:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM=7 go build .
```

In case of the `mips` architecture, there's an additional flag available, `GOMIPS`, that allows to set additional parameters:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build .
```

The command will produce the `mediamtx` binary.

### Compile for all supported platforms

Install Docker and launch:

```sh
make binaries
```

The command will produce tarballs in folder `binaries/`.

## License

All the code in this repository is released under the [MIT License](LICENSE). Compiled binaries make use of some third-party dependencies:

* hls.js, released under the [Apache License 2.0](https://github.com/video-dev/hls.js/blob/master/LICENSE)
* all the dependencies listed into the [go.mod file](go.mod), which are all released under either the MIT license, BSD-3-Clause license or Apache License 2.0

## Specifications

|name|area|
|----|----|
|[RTSP / RTP / RTCP specifications](https://github.com/bluenviron/gortsplib#specifications)|RTSP|
|[HLS specifications](https://github.com/bluenviron/gohlslib#specifications)|HLS|
|[Action Message Format - AMF 0](https://veovera.org/docs/legacy/amf0-file-format-spec.pdf)|RTMP|
|[FLV](https://veovera.org/docs/legacy/video-file-format-v10-1-spec.pdf)|RTMP|
|[RTMP](https://veovera.org/docs/legacy/rtmp-v1-0-spec.pdf)|RTMP|
|[Enhanced RTMP v2](https://veovera.org/docs/enhanced/enhanced-rtmp-v2.pdf)|RTMP|
|[WebRTC: Real-Time Communication in Browsers](https://www.w3.org/TR/webrtc/)|WebRTC|
|[RFC8835, Transports for WebRTC](https://datatracker.ietf.org/doc/html/rfc8835)|WebRTC|
|[RFC7742, WebRTC Video Processing and Codec Requirements](https://datatracker.ietf.org/doc/html/rfc7742)|WebRTC|
|[RFC7847, WebRTC Audio Codec and Processing Requirements](https://datatracker.ietf.org/doc/html/rfc7874)|WebRTC|
|[RFC7875, Additional WebRTC Audio Codecs for Interoperability](https://datatracker.ietf.org/doc/html/rfc7875)|WebRTC|
|[H.265 Profile for WebRTC](https://datatracker.ietf.org/doc/draft-ietf-avtcore-hevc-webrtc/)|WebRTC|
|[WebRTC HTTP Ingestion Protocol (WHIP)](https://datatracker.ietf.org/doc/draft-ietf-wish-whip/)|WebRTC|
|[WebRTC HTTP Egress Protocol (WHEP)](https://datatracker.ietf.org/doc/draft-murillo-whep/)|WebRTC|
|[The SRT Protocol](https://haivision.github.io/srt-rfc/draft-sharabayko-srt.html)|SRT|
|[Codec specifications](https://github.com/bluenviron/mediacommon#specifications)|codecs|
|[Golang project layout](https://github.com/golang-standards/project-layout)|project layout|

## Related projects

* [gortsplib (RTSP library used internally)](https://github.com/bluenviron/gortsplib)
* [gohlslib (HLS library used internally)](https://github.com/bluenviron/gohlslib)
* [mediacommon (codecs and formats library used internally)](https://github.com/bluenviron/mediacommon)
* [mediamtx-rpicamera (Raspberry Pi Camera component)](https://github.com/bluenviron/mediamtx-rpicamera)
* [datarhei/gosrt (SRT library used internally)](https://github.com/datarhei/gosrt)
* [pion/webrtc (WebRTC library used internally)](https://github.com/pion/webrtc)
* [pion/sdp (SDP library used internally)](https://github.com/pion/sdp)
* [pion/rtp (RTP library used internally)](https://github.com/pion/rtp)
* [pion/rtcp (RTCP library used internally)](https://github.com/pion/rtcp)
* [go-astits (MPEG-TS library used internally)](https://github.com/asticode/go-astits)
* [go-mp4 (MP4 library used internally)](https://github.com/abema/go-mp4)
* [hls.js (browser-side HLS library used internally)](https://github.com/video-dev/hls.js)
