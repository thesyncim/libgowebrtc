package pc

func testPeerConnectionConfig() Configuration {
	return Configuration{
		BundlePolicy:       BundlePolicyBalanced,
		RTCPMuxPolicy:      RTCPMuxPolicyRequire,
		ICETransportPolicy: ICETransportPolicyAll,
		SDPSemantics:       SDPSemanticsUnifiedPlan,
		ICEServers:         nil,
	}
}
