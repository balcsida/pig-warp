package pig_warp

// Test seams for the external (fake-host) tests.

var (
	WriteRaw      = &writeRaw
	IdleDelay     = &idleDelay
	FrameInterval = &frameInterval
	HostCommand   = &hostCommand
)

const (
	PushTitleStack = pushTitleStack
	PopTitleStack  = popTitleStack
	StatusKey      = statusKey
	LaunchType     = launchType
	AppliedType    = launchAppliedType
)
