// Package shared provides the flags that more than one command accepts.
//
// A command lists its own flags in its package and takes each flag it has in common with another command
// from here, by calling the constructor for that flag or group of flags. It takes a flag only when the
// code it runs reads the option the flag sets, so a flag the command accepts always has an effect.
//
// No command builds its flags from another command's.
package shared
