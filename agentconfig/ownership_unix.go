// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows

package agentconfig

import (
	"os"
	"os/user"
	"strconv"
)

type fileOwnership struct {
	uid int
	gid int
}

func ownershipForOwner(owner string) (fileOwnership, error) {
	var account *user.User
	var err error
	if owner == "" {
		account, err = user.Current()
	} else {
		account, err = user.Lookup(owner)
	}
	if err != nil {
		return fileOwnership{}, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return fileOwnership{}, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return fileOwnership{}, err
	}
	return fileOwnership{uid: uid, gid: gid}, nil
}

func applyOwnership(path string, ownership fileOwnership) error {
	return os.Chown(path, ownership.uid, ownership.gid)
}
