package cli

// Export internal functions for testing.

// RunRecord exports runRecord for testing.
var RunRecord = runRecord

// RunTranscribe exports runTranscribe for testing.
// Note: This requires passing a *cobra.Command; consider refactoring if this becomes cumbersome.

// RunLive exports runLive for testing.
var RunLive = runLive

// RunConfigSet exports runConfigSet for testing.
var RunConfigSet = runConfigSet

// RunConfigGet exports runConfigGet for testing.
var RunConfigGet = runConfigGet

// RunConfigList exports runConfigList for testing.
var RunConfigList = runConfigList

// ClampParallel exports clampParallel for testing.
var ClampParallel = clampParallel

// DeriveOutputPath exports deriveOutputPath for testing.
var DeriveOutputPath = deriveOutputPath

// SupportedFormatsList exports supportedFormatsList for testing.
var SupportedFormatsList = supportedFormatsList

// DefaultRecordingFilename exports defaultRecordingFilename for testing.
var DefaultRecordingFilename = defaultRecordingFilename

// DefaultLiveFilename exports defaultLiveFilename for testing.
var DefaultLiveFilename = defaultLiveFilename

// AudioOutputPath exports audioOutputPath for testing.
var AudioOutputPath = audioOutputPath

// IsValidConfigKey exports isValidConfigKey for testing.
var IsValidConfigKey = isValidConfigKey

// ValidConfigKeys exports validConfigKeys for testing.
var ValidConfigKeys = validConfigKeys

// MoveFile exports moveFile for testing.
var MoveFile = moveFile

// CopyFile exports copyFile for testing.
var CopyFile = copyFile

// FileSize exports fileSize for testing.
var FileSize = fileSize

// LiveWritePhase exports liveWritePhase for testing.
var LiveWritePhase = liveWritePhase
