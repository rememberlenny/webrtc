// +build !js

package webrtc

import (
	"io"
	"sync"

	"github.com/pion/rtcp"
	"github.com/pion/srtp"
)

// RTPSender allows an application to control how a given Track is encoded and transmitted to a remote peer
type RTPSender struct {
	track          TrackLocal
	rtcpReadStream *srtp.ReadStreamSRTCP

	transport *DTLSTransport

	// TODO(Sean-Der)
	payloadType uint8
	ssrc        uint32

	// nolint:godox
	// TODO(sgotti) remove this when in future we'll avoid replacing
	// a transceiver sender since we can just check the
	// transceiver negotiation status
	negotiated bool

	// A reference to the associated api object
	api *API

	mu                     sync.RWMutex
	sendCalled, stopCalled chan interface{}
}

// NewRTPSender constructs a new RTPSender
func (api *API) NewRTPSender(track TrackLocal, transport *DTLSTransport) (*RTPSender, error) {
	if track == nil {
		return nil, errRTPSenderTrackNil
	} else if transport == nil {
		return nil, errRTPSenderDTLSTransportNil
	}

	return &RTPSender{
		track:      track,
		transport:  transport,
		api:        api,
		sendCalled: make(chan interface{}),
		stopCalled: make(chan interface{}),
	}, nil
}

func (r *RTPSender) isNegotiated() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.negotiated
}

func (r *RTPSender) setNegotiated() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.negotiated = true
}

// Transport returns the currently-configured *DTLSTransport or nil
// if one has not yet been configured
func (r *RTPSender) Transport() *DTLSTransport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.transport
}

// Track returns the RTCRtpTransceiver track, or nil
func (r *RTPSender) Track() TrackLocal {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.track
}

func (r *RTPSender) setTrack(track TrackLocal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.track = track
}

// Send Attempts to set the parameters controlling the sending of media.
func (r *RTPSender) Send(parameters RTPSendParameters) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hasSent() {
		return errRTPSenderSendAlreadyCalled
	}

	srtcpSession, err := r.transport.getSRTCPSession()
	if err != nil {
		return err
	}

	r.rtcpReadStream, err = srtcpSession.OpenReadStream(parameters.Encodings.SSRC)
	if err != nil {
		return err
	}

	// r.track.mu.Lock()
	// r.track.activeSenders = append(r.track.activeSenders, r)
	// r.track.mu.Unlock()

	close(r.sendCalled)
	return nil
}

// Stop irreversibly stops the RTPSender
func (r *RTPSender) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.stopCalled:
		return nil
	default:
	}

	// r.track.mu.Lock()
	// defer r.track.mu.Unlock()
	// filtered := []*RTPSender{}
	// for _, s := range r.track.activeSenders {
	// 	if s != r {
	// 		filtered = append(filtered, s)
	// 	} else {
	// 		r.track.totalSenderCount--
	// 	}
	// }
	// r.track.activeSenders = filtered
	// close(r.stopCalled)

	if r.hasSent() {
		return r.rtcpReadStream.Close()
	}

	return nil
}

// Read reads incoming RTCP for this RTPReceiver
func (r *RTPSender) Read(b []byte) (n int, err error) {
	select {
	case <-r.sendCalled:
		return r.rtcpReadStream.Read(b)
	case <-r.stopCalled:
		return 0, io.ErrClosedPipe
	}
}

// ReadRTCP is a convenience method that wraps Read and unmarshals for you
func (r *RTPSender) ReadRTCP() ([]rtcp.Packet, error) {
	b := make([]byte, receiveMTU)
	i, err := r.Read(b)
	if err != nil {
		return nil, err
	}

	return rtcp.Unmarshal(b[:i])
}

// hasSent tells if data has been ever sent for this instance
func (r *RTPSender) hasSent() bool {
	select {
	case <-r.sendCalled:
		return true
	default:
		return false
	}
}
