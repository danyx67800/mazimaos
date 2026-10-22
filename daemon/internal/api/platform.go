package api

import "runtime"

func runtimeIsLinux() bool { return runtime.GOOS == "linux" }
