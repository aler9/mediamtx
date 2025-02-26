'use strict';

(() => {

  const unquoteCredential = (v) => (
    JSON.parse(`"${v}"`)
  );

  const linkToIceServers = (links) => (
    (links !== null) ? links.split(', ').map((link) => {
      const m = link.match(/^<(.+?)>; rel="ice-server"(; username="(.*?)"; credential="(.*?)"; credential-type="password")?/i);
      const ret = {
        urls: [m[1]],
      };

      if (m[3] !== undefined) {
        ret.username = unquoteCredential(m[3]);
        ret.credential = unquoteCredential(m[4]);
        ret.credentialType = 'password';
      }

      return ret;
    }) : []
  );

  const parseOffer = (offer) => {
    const ret = {
      iceUfrag: '',
      icePwd: '',
      medias: [],
    };

    for (const line of offer.split('\r\n')) {
      if (line.startsWith('m=')) {
        ret.medias.push(line.slice('m='.length));
      } else if (ret.iceUfrag === '' && line.startsWith('a=ice-ufrag:')) {
        ret.iceUfrag = line.slice('a=ice-ufrag:'.length);
      } else if (ret.icePwd === '' && line.startsWith('a=ice-pwd:')) {
        ret.icePwd = line.slice('a=ice-pwd:'.length);
      }
    }

    return ret;
  };

  const generateSdpFragment = (od, candidates) => {
    const candidatesByMedia = {};
    for (const candidate of candidates) {
      const mid = candidate.sdpMLineIndex;
      if (candidatesByMedia[mid] === undefined) {
        candidatesByMedia[mid] = [];
      }
      candidatesByMedia[mid].push(candidate);
    }

    let frag = 'a=ice-ufrag:' + od.iceUfrag + '\r\n'
      + 'a=ice-pwd:' + od.icePwd + '\r\n';

    let mid = 0;

    for (const media of od.medias) {
      if (candidatesByMedia[mid] !== undefined) {
        frag += 'm=' + media + '\r\n'
          + 'a=mid:' + mid + '\r\n';

        for (const candidate of candidatesByMedia[mid]) {
          frag += 'a=' + candidate.candidate + '\r\n';
        }
      }
      mid++;
    }

    return frag;
  };

  const setCodec = (section, codec) => {
    const lines = section.split('\r\n');
    const lines2 = [];
    const payloadFormats = [];

    for (const line of lines) {
      if (!line.startsWith('a=rtpmap:')) {
        lines2.push(line);
      } else {
        if (line.toLowerCase().includes(codec)) {
          payloadFormats.push(line.slice('a=rtpmap:'.length).split(' ')[0]);
          lines2.push(line);
        }
      }
    }

    const lines3 = [];
    let firstLine = true;

    for (const line of lines2) {
      if (firstLine) {
        firstLine = false;
        lines3.push(line.split(' ').slice(0, 3).concat(payloadFormats).join(' '));
      } else if (line.startsWith('a=fmtp:')) {
        if (payloadFormats.includes(line.slice('a=fmtp:'.length).split(' ')[0])) {
          lines3.push(line);
        }
      } else if (line.startsWith('a=rtcp-fb:')) {
        if (payloadFormats.includes(line.slice('a=rtcp-fb:'.length).split(' ')[0])) {
          lines3.push(line);
        }
      } else {
        lines3.push(line);
      }
    }

    return lines3.join('\r\n');
  };

  const setVideoBitrate = (section, bitrate) => {
    let lines = section.split('\r\n');

    for (let i = 0; i < lines.length; i++) {
      if (lines[i].startsWith('c=')) {
        lines = [...lines.slice(0, i+1), 'b=TIAS:' + (parseInt(bitrate) * 1024).toString(), ...lines.slice(i+1)];
        break
      }
    }

    return lines.join('\r\n');
  };

  const setAudioBitrate = (section, bitrate, voice) => {
    let opusPayloadFormat = '';
    let lines = section.split('\r\n');

    for (let i = 0; i < lines.length; i++) {
      if (lines[i].startsWith('a=rtpmap:') && lines[i].toLowerCase().includes('opus/')) {
        opusPayloadFormat = lines[i].slice('a=rtpmap:'.length).split(' ')[0];
        break;
      }
    }

    if (opusPayloadFormat === '') {
      return section;
    }

    for (let i = 0; i < lines.length; i++) {
      if (lines[i].startsWith('a=fmtp:' + opusPayloadFormat + ' ')) {
        if (voice) {
          lines[i] = 'a=fmtp:' + opusPayloadFormat + ' minptime=10;useinbandfec=1;maxaveragebitrate='
            + (parseInt(bitrate) * 1024).toString();
        } else {
          lines[i] = 'a=fmtp:' + opusPayloadFormat + ' maxplaybackrate=48000;stereo=1;sprop-stereo=1;maxaveragebitrate='
            + (parseInt(bitrate) * 1024).toString();
        }
      }
    }

    return lines.join('\r\n');
  };

  const editOffer = (sdp, videoCodec, audioCodec, audioBitrate, audioVoice) => {
    const sections = sdp.split('m=');

    for (let i = 0; i < sections.length; i++) {
      if (sections[i].startsWith('video')) {
        sections[i] = setCodec(sections[i], videoCodec);
      } else if (sections[i].startsWith('audio')) {
        sections[i] = setAudioBitrate(setCodec(sections[i], audioCodec), audioBitrate, audioVoice);
      }
    }

    return sections.join('m=');
  };

  const editAnswer = (sdp, videoBitrate) => {
    const sections = sdp.split('m=');

    for (let i = 0; i < sections.length; i++) {
      if (sections[i].startsWith('video')) {
        sections[i] = setVideoBitrate(sections[i], videoBitrate);
      }
    }

    return sections.join('m=');
  };

  const retryPause = 2000;

  class MediaMTXWebRTCPublisher {
    constructor(conf) {
      this.conf = conf;
      this.state = 'initializing';
      this.restartTimeout = null;
      this.pc = null;
      this.offerData = null;
      this.sessionUrl = null;
      this.queuedCandidates = [];

      this.start();
    }

    start = () => {
      this.state = 'running';

      this.requestICEServers()
        .then((iceServers) => this.setupPeerConnection(iceServers))
        .then((offer) => this.sendOffer(offer))
        .then((answer) => this.setAnswer(answer))
        .catch((err) => {
          this.handleError(err.toString());
        });
    };

    handleError = (err) => {
      if (this.state === 'restarting' || this.state === 'error') {
        return;
      }

      if (this.pc !== null) {
        this.pc.close();
        this.pc = null;
      }

      this.offerData = null;

      if (this.sessionUrl !== null) {
        fetch(this.sessionUrl, {
          method: 'DELETE',
        });
        this.sessionUrl = null;
      }

      this.queuedCandidates = [];

      if (this.state === 'running') {
        this.state = 'restarting';

        this.restartTimeout = window.setTimeout(() => {
          this.restartTimeout = null;
          this.start();
        }, retryPause);

        if (this.conf.onError !== undefined) {
          this.conf.onError(err + ', retrying in some seconds');
        }
      } else {
        this.state = 'error';

        if (this.conf.onError !== undefined) {
          this.conf.onError(err);
        }
      }
    };

    requestICEServers = () => {
      return fetch(this.conf.url, {
        method: 'OPTIONS',
      })
        .then((res) => linkToIceServers(res.headers.get('Link')));
    };

    setupPeerConnection = (iceServers) => {
      this.pc = new RTCPeerConnection({
        iceServers,
        // https://webrtc.org/getting-started/unified-plan-transition-guide
        sdpSemantics: 'unified-plan',
      });

      this.pc.onicecandidate = (evt) => this.onLocalCandidate(evt);
      this.pc.onconnectionstatechange = () => this.onConnectionState();

      this.conf.stream.getTracks().forEach((track) => {
        this.pc.addTrack(track, this.conf.stream);
      });

      return this.pc.createOffer()
        .then((offer) => {
          this.offerData = parseOffer(offer.sdp);

          return this.pc.setLocalDescription(offer)
            .then(() => offer.sdp);
        });
    };

    sendOffer = (offer) => {
      offer = editOffer(
        offer,
        this.conf.videoCodec,
        this.conf.audioCodec,
        this.conf.audioBitrate,
        this.conf.audioVoice);

      return fetch(this.conf.url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/sdp',
        },
        body: offer,
      })
        .then((res) => {
          switch (res.status) {
          case 201:
            break;
          case 400:
            return res.json().then((e) => { throw new Error(e.error); });
          default:
            throw new Error(`bad status code ${res.status}`);
          }

          this.sessionUrl = new URL(res.headers.get('location'), this.conf.url).toString();

          return res.text();
        });
    };

    setAnswer = (answer) => {
      if (this.state !== 'running') {
        return;
      }

      answer = editAnswer(answer, this.conf.videoBitrate);

      return this.pc.setRemoteDescription(new RTCSessionDescription({
        type: 'answer',
        sdp: answer,
      }))
        .then(() => {
          if (this.queuedCandidates.length !== 0) {
            this.sendLocalCandidates(this.queuedCandidates);
            this.queuedCandidates = [];
          }
        });
    };

    onLocalCandidate = (evt) => {
      if (this.state !== 'running') {
        return;
      }

      if (evt.candidate !== null) {
        if (this.sessionUrl === null) {
          this.queuedCandidates.push(evt.candidate);
        } else {
          this.sendLocalCandidates([evt.candidate]);
        }
      }
    };

    sendLocalCandidates = (candidates) => {
      fetch(this.sessionUrl, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/trickle-ice-sdpfrag',
          'If-Match': '*',
        },
        body: generateSdpFragment(this.offerData, candidates),
      })
        .then((res) => {
          switch (res.status) {
          case 204:
            break;
          case 404:
            throw new Error('stream not found');
          default:
            throw new Error(`bad status code ${res.status}`);
          }
        })
        .catch((err) => {
          this.handleError(err.toString());
        });
    };

    onConnectionState = () => {
      if (this.state !== 'running') {
        return;
      }

      // "closed" can arrive before "failed" and without
      // the close() method being called at all.
      // It happens when the other peer sends a termination
      // message like a DTLS CloseNotify.
      if (this.pc.connectionState === 'failed'
        || this.pc.connectionState === 'closed'
      ) {
        this.handleError('peer connection closed');
      } else if (this.pc.connectionState === 'connected') {
        if (this.conf.onConnected !== undefined) {
          this.conf.onConnected();
        }
      }
    };

  }

  window.MediaMTXWebRTCPublisher = MediaMTXWebRTCPublisher;

})();
