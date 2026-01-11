package ffi

// shimSessionDescription matches ShimSessionDescription in shim.h.
type shimSessionDescription struct {
	Type int32
	SDP  *byte
}

// shimICECandidate matches ShimICECandidate in shim.h.
type shimICECandidate struct {
	Candidate     *byte
	SDPMid        *byte
	SDPMLineIndex int32
}
