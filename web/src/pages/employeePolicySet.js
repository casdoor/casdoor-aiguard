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

// Shared by the three pages that work with a person's own policy: the Digital
// Employee page, which writes the set they authored; the Self-Learning page,
// which shows the set aiguard derived from their corrections; and the Policy
// Fusion page, which reads both back to combine them with an agent's. All of
// them need the same thing first: the signed-in person, or the reason there
// isn't one - these sets belong to a person, so unlike the rest of aiguard
// these pages have nothing to show an anonymous session.

import React, {useCallback, useEffect, useState} from "react";
import {Button, Empty, Typography} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import {getEmployeePolicySet, getLearnedPolicySet} from "../backend/api";

const {Text, Paragraph} = Typography;

// Both of a person's policy sets - the one they wrote and the one aiguard
// learned from their corrections - load the same way, so one hook serves both.
function usePersonalPolicySet(load) {
  const [policySet, setPolicySet] = useState(null);
  const [loadError, setLoadError] = useState(null);

  const reload = useCallback(() => {
    load()
      .then((data) => {
        setPolicySet(data);
        setLoadError(null);
      })
      .catch((err) => setLoadError(err.message));
  }, [load]);

  useEffect(reload, [reload]);

  return {policySet, loadError, reload, setPolicySet};
}

export function useEmployeePolicySet() {
  return usePersonalPolicySet(getEmployeePolicySet);
}

// The self-learned set: the Casbin rules derived from the records this person
// marked as wrongly decided. It is read wherever the employee's set is, because
// the two together are what this person's policy actually is.
export function useLearnedPolicySet() {
  return usePersonalPolicySet(getLearnedPolicySet);
}

// EmployeeUnavailable explains why there is no policy set to work with, which
// is almost always "nobody is signed in" rather than a real failure.
export function EmployeeUnavailable({error, account, onRetry}) {
  return (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={
        <div style={{maxWidth: 460, margin: "0 auto"}}>
          <Paragraph style={{marginBottom: 4}}>{error}</Paragraph>
          <Text type="secondary">
            A digital employee belongs to a person, so this page needs a signed-in Casdoor user
            {account ? "." : " - use the Login button in the top right."}
          </Text>
        </div>
      }
      style={{padding: "60px 0"}}
    >
      <Button onClick={onRetry} icon={<ReloadOutlined />}>Retry</Button>
    </Empty>
  );
}
