//go:build !windows && !linux && !darwin

package activity

// Windows returns nothing, as discordo cannot see the programs that run here.
func Windows() []Window { return nil }
