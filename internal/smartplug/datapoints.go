package smartplug

// Data-point IDs for the lsc-3202087-2 profile. Tuya encodes these numeric
// identifiers as JSON object keys, so the constants use their wire strings.
const (
	dpRelay                = "1"
	dpCountdownSeconds     = "9"
	dpEnergyReportWh       = "17"
	dpCurrent              = "18"
	dpPower                = "19"
	dpVoltage              = "20"
	dpFault                = "26"
	dpPowerOnState         = "38"
	dpOverchargeProtection = "39"
	dpIndicatorMode        = "40"
	dpChildLock            = "41"
)
