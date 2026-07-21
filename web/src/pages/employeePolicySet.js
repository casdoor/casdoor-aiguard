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

// Shared between the Digital Employee page, which writes a person's policy set,
// and the Policy Fusion page, which reads it back to combine with an agent's.
// Both need the same thing first: the signed-in person's set, or the reason
// there isn't one - a digital employee belongs to a person, so unlike the rest
// of aiguard these two pages have nothing to show an anonymous session.

import React, {useCallback, useEffect, useState} from "react";
import {Button, Empty, Typography} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import {getEmployeePolicySet} from "../backend/api";

const {Text, Paragraph} = Typography;

export function useEmployeePolicySet() {
  const [policySet, setPolicySet] = useState(null);
  const [loadError, setLoadError] = useState(null);

  const reload = useCallback(() => {
    getEmployeePolicySet()
      .then((data) => {
        setPolicySet(data);
        setLoadError(null);
      })
      .catch((err) => setLoadError(err.message));
  }, []);

  useEffect(reload, [reload]);

  return {policySet, loadError, reload, setPolicySet};
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
