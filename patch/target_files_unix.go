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

package patch

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
)

func setTargetFileOwner(path string, target Target) error {
	if target.Owner == "" || isCurrentUser(target.Owner) {
		return nil
	}
	account, err := user.Lookup(target.Owner)
	if err != nil {
		return fmt.Errorf("cannot resolve target user %q: %w", target.Owner, err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid < 0 {
		return fmt.Errorf("target user %q has invalid uid %q", target.Owner, account.Uid)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid < 0 {
		return fmt.Errorf("target user %q has invalid gid %q", target.Owner, account.Gid)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("cannot set owner of %s: %w", path, err)
	}
	return nil
}
