//go:build !windows

package main

// fatalDialog is a no-op on non-Windows builds — those keep a console
// attached (no -H=windowsgui-style console suppression), so the stderr
// print already at the fatalDialog call site is already visible there.
func fatalDialog(title, message string) {}
