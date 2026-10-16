//go:build !darwin || ios

package browser

const canDictate = false

func startDictation() {}

func refocus() {}
