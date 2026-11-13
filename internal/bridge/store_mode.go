//go:build !appstore

package bridge

const appStoreBuild = false

func prepareStoreOptions(options *Options)          {}
func acquireClientAccess(home, client string) error { return nil }
