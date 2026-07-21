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

import React, {useCallback, useEffect, useState} from "react";
import {Layout, Menu, Tag, Tooltip} from "antd";
import {DashboardOutlined, RobotOutlined, SafetyCertificateOutlined, SettingOutlined, ApiOutlined, FileTextOutlined, DesktopOutlined, IdcardOutlined, MergeCellsOutlined} from "@ant-design/icons";
import {Link, Route, Switch, useLocation} from "react-router-dom";
import DashboardPage from "./pages/DashboardPage";
import PolicyHubPage from "./pages/PolicyHubPage";
import PolicySetPage from "./pages/PolicySetPage";
import DigitalEmployeePage from "./pages/DigitalEmployeePage";
import PolicyFusionPage from "./pages/PolicyFusionPage";
import InterceptPage from "./pages/InterceptPage";
import CasdoorSettingsPage from "./pages/CasdoorSettingsPage";
import AgentsPage from "./pages/AgentsPage";
import RecordsPage from "./pages/RecordsPage";
import AccountArea from "./auth/AccountArea";
import AuthCallback from "./auth/AuthCallback";
import {getAccount, getAuthConfig, getHostInfo} from "./backend/api";
import {CasdoorLogo} from "./theme";

const {Header, Sider, Content, Footer} = Layout;

const headerHeight = 56;

const menuItems = [
  {key: "/", icon: <DashboardOutlined />, label: <Link to="/">Dashboard</Link>},
  {key: "/agents", icon: <RobotOutlined />, label: <Link to="/agents">Agents</Link>},
  {key: "/records", icon: <FileTextOutlined />, label: <Link to="/records">Records</Link>},
  {key: "/policyhub", icon: <SafetyCertificateOutlined />, label: <Link to="/policyhub">Policy Hub</Link>},
  {key: "/digital-employee", icon: <IdcardOutlined />, label: <Link to="/digital-employee">Digital Employee</Link>},
  {key: "/policy-fusion", icon: <MergeCellsOutlined />, label: <Link to="/policy-fusion">Policy Fusion</Link>},
  {key: "/intercept", icon: <ApiOutlined />, label: <Link to="/intercept">Interception</Link>},
  {key: "/casdoor", icon: <SettingOutlined />, label: <Link to="/casdoor">Settings</Link>},
];

// A policy set lives under /policyhub/<name>, so the menu highlights the
// deepest item whose path the current URL sits inside rather than an exact match.
function selectedMenuKey(pathname) {
  const match = menuItems
    .filter((item) => item.key !== "/" && (pathname === item.key || pathname.startsWith(`${item.key}/`)))
    .sort((a, b) => b.key.length - a.key.length)[0];
  return match ? match.key : "/";
}

function App() {
  const location = useLocation();
  // null means "anonymous" for the account and "not loaded yet" for the config;
  // login is optional, so neither one blocks the pages from rendering.
  const [authConfig, setAuthConfig] = useState(null);
  const [account, setAccount] = useState(null);
  // The host aiguard is installed on, shown in the top bar so it is clear that
  // this instance guards this machine.
  const [hostInfo, setHostInfo] = useState(null);

  useEffect(() => {
    getAuthConfig().then(setAuthConfig).catch(() => setAuthConfig(null));
    getAccount().then(setAccount).catch(() => setAccount(null));
    getHostInfo().then(setHostInfo).catch(() => setHostInfo(null));
  }, []);

  const onSignedIn = useCallback((claims) => setAccount(claims), []);

  return (
    <Layout style={{minHeight: "100vh"}}>
      <Header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          height: headerHeight,
          lineHeight: `${headerHeight}px`,
          padding: "0 24px",
          background: "#fff",
          boxShadow: "0 1px 4px rgba(0, 0, 0, 0.08)",
          position: "sticky",
          top: 0,
          zIndex: 99,
        }}
      >
        <Link to="/" style={{display: "flex", alignItems: "center", gap: 10}}>
          <img src={CasdoorLogo} alt="Casdoor" style={{height: 26, objectFit: "contain"}} />
          <span style={{color: "#262626", fontSize: 18, fontWeight: 600, letterSpacing: 0.2}}>AIGuard</span>
        </Link>
        <div style={{display: "flex", alignItems: "center", gap: 16}}>
          {hostInfo &&
            <Tooltip title={`AIGuard is protecting this machine (${hostInfo.os}/${hostInfo.arch})`}>
              <Tag icon={<DesktopOutlined />} color="blue" style={{marginInlineEnd: 0, fontSize: 13, padding: "2px 10px", borderRadius: 14}}>
                {hostInfo.hostname}
              </Tag>
            </Tooltip>
          }
          <AccountArea authConfig={authConfig} account={account} onSignedOut={() => setAccount(null)} />
        </div>
      </Header>
      <Layout>
        <Sider
          width={220}
          theme="light"
          breakpoint="lg"
          collapsedWidth={0}
          style={{
            position: "sticky",
            top: headerHeight,
            height: `calc(100vh - ${headerHeight}px)`,
            overflow: "auto",
            borderRight: "1px solid #f0f0f0",
          }}
        >
          <Menu mode="inline" selectedKeys={[selectedMenuKey(location.pathname)]} items={menuItems} style={{height: "100%", borderRight: 0, paddingTop: 8}} />
        </Sider>
        <Layout style={{background: "#fafafa"}}>
          <Content style={{padding: 24}}>
            <div style={{background: "#fff", borderRadius: 8, border: "1px solid #f0f0f0", boxShadow: "0 1px 5px 0 rgba(51, 51, 51, 0.06)", padding: 24, minHeight: "100%"}}>
              <Switch>
                <Route exact path="/" component={DashboardPage} />
                <Route exact path="/agents" component={AgentsPage} />
                <Route exact path="/records" component={RecordsPage} />
                <Route exact path="/policyhub" component={PolicyHubPage} />
                <Route exact path="/policyhub/:name" component={PolicySetPage} />
                <Route exact path="/digital-employee" render={() => <DigitalEmployeePage account={account} />} />
                <Route exact path="/policy-fusion" render={() => <PolicyFusionPage account={account} />} />
                <Route exact path="/intercept" component={InterceptPage} />
                <Route exact path="/casdoor" component={CasdoorSettingsPage} />
                <Route exact path="/callback" render={() => <AuthCallback onSignedIn={onSignedIn} />} />
              </Switch>
            </div>
          </Content>
          <Footer style={{display: "flex", alignItems: "center", justifyContent: "center", gap: 6, background: "transparent", color: "#737373", padding: "16px 24px 24px"}}>
            Powered by
            <a target="_blank" href="https://casdoor.org" rel="noreferrer" style={{display: "inline-flex", alignItems: "center"}}>
              <img height={20} alt="Casdoor" src={CasdoorLogo} />
            </a>
          </Footer>
        </Layout>
      </Layout>
    </Layout>
  );
}

export default App;
