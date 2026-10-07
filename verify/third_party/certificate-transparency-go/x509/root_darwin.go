// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package x509

import (
	"errors"
	"os"
	"strings"
)

var debugDarwinRoots = strings.Contains(os.Getenv("GODEBUG"), "x509roots=1")

func (c *Certificate) systemVerify(opts *VerifyOptions) (chains [][]*Certificate, err error) {
	return nil, nil
}

// execSecurityRoots has no system roots to offer: this copy starts no process.
func execSecurityRoots() (*CertPool, error) {
	return nil, errors.New("x509: system roots without cgo are unavailable")
}
