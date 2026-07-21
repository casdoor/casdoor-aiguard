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

// Shadcn-style Ant Design theme, kept in sync with Casdoor's own web UI so the
// two consoles look like one product. Casdoor also ships a dark variant; aiguard
// has no theme switcher yet, so only the light preset lives here.

export const StaticBaseUrl = "https://cdn.casbin.org";

export const CasdoorLogo = `${StaticBaseUrl}/img/casdoor-logo_1185x256.png`;

export const themeToken = {
  fontFamily: "'Inter Variable', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif",
  colorPrimary: "#262626",
  colorSuccess: "#22c55e",
  colorWarning: "#f97316",
  colorError: "#ef4444",
  colorInfo: "#262626",
  // The info color is near-black like the primary one, and Ant Design derives
  // the info background and border from it, which comes out dark gray behind an
  // info Alert. Pin the shadcn neutrals the rest of the theme already uses.
  colorInfoBg: "#f4f4f4",
  colorInfoBgHover: "#ebebeb",
  colorInfoBorder: "#e4e4e7",
  colorInfoBorderHover: "#d4d4d8",
  colorInfoHover: "#404040",
  colorInfoActive: "#171717",
  colorInfoText: "#262626",
  colorInfoTextHover: "#404040",
  colorInfoTextActive: "#171717",
  colorTextBase: "#262626",
  colorBgBase: "#ffffff",
  colorSuccessBg: "#f0fdf4",
  colorSuccessBgHover: "#dcfce7",
  colorSuccessBorder: "#bbf7d0",
  colorSuccessBorderHover: "#86efac",
  colorSuccessHover: "#16a34a",
  colorSuccessActive: "#15803d",
  colorSuccessText: "#16a34a",
  colorSuccessTextHover: "#16a34a",
  colorSuccessTextActive: "#15803d",
  colorWarningBg: "#fff7ed",
  colorWarningBgHover: "#fed7aa",
  colorWarningBorder: "#fdba74",
  colorWarningBorderHover: "#fb923c",
  colorWarningHover: "#ea580c",
  colorWarningActive: "#c2410c",
  colorWarningText: "#ea580c",
  colorWarningTextHover: "#ea580c",
  colorWarningTextActive: "#c2410c",
  colorErrorBg: "#fef2f2",
  colorErrorBgHover: "#fecaca",
  colorErrorBorder: "#fca5a5",
  colorErrorBorderHover: "#f87171",
  colorErrorHover: "#dc2626",
  colorErrorActive: "#b91c1c",
  colorErrorText: "#dc2626",
  colorErrorTextHover: "#dc2626",
  colorErrorTextActive: "#b91c1c",
  colorLink: "#262626",
  colorText: "#262626",
  colorTextSecondary: "#525252",
  colorTextTertiary: "#737373",
  colorTextQuaternary: "#a3a3a3",
  colorTextDisabled: "#a3a3a3",
  colorBgContainer: "#ffffff",
  colorBgElevated: "#ffffff",
  colorBgLayout: "#fafafa",
  colorBgSpotlight: "rgba(38, 38, 38, 0.85)",
  colorBgMask: "rgba(38, 38, 38, 0.45)",
  colorBorder: "#e5e5e5",
  colorBorderSecondary: "#f5f5f5",
  borderRadius: 10,
  borderRadiusXS: 2,
  borderRadiusSM: 6,
  borderRadiusLG: 14,
  padding: 16,
  paddingSM: 12,
  paddingLG: 24,
  margin: 16,
  marginSM: 12,
  marginLG: 24,
  boxShadow: "0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px -1px rgba(0, 0, 0, 0.1)",
  boxShadowSecondary: "0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -2px rgba(0, 0, 0, 0.1)",
};

export const themeComponents = {
  Button: {
    primaryShadow: "none",
    defaultShadow: "none",
    dangerShadow: "none",
    defaultBorderColor: "#e4e4e7",
    defaultColor: "#18181b",
    defaultBg: "#ffffff",
    defaultHoverBg: "#f4f4f5",
    defaultHoverBorderColor: "#d4d4d8",
    defaultHoverColor: "#18181b",
    defaultActiveBg: "#e4e4e7",
    defaultActiveBorderColor: "#d4d4d8",
    borderRadius: 6,
  },
  Input: {
    activeShadow: "none",
    hoverBorderColor: "#a1a1aa",
    activeBorderColor: "#18181b",
    borderRadius: 6,
  },
  Select: {
    optionSelectedBg: "#f4f4f5",
    optionActiveBg: "#fafafa",
    optionSelectedFontWeight: 500,
    borderRadius: 6,
  },
  Alert: {
    borderRadiusLG: 8,
  },
  Modal: {
    borderRadiusLG: 12,
  },
  Progress: {
    defaultColor: "#18181b",
    remainingColor: "#f4f4f5",
  },
  Steps: {
    iconSize: 32,
  },
  Switch: {
    trackHeight: 24,
    trackMinWidth: 44,
    innerMinMargin: 4,
    innerMaxMargin: 24,
  },
  Checkbox: {
    borderRadiusSM: 4,
  },
  Slider: {
    trackBg: "#f4f4f5",
    trackHoverBg: "#e4e4e7",
    handleSize: 18,
    handleSizeHover: 20,
    railSize: 6,
  },
  Menu: {
    itemFontSize: 14,
    groupTitleFontSize: 12,
    itemHeight: 40,
    fontWeightStrong: 600,
    // The primary color is near-black, and Ant Design derives the selected
    // background from it, which comes out far too dark to read against. Pin the
    // shadcn accent gray instead and keep the label in the primary color.
    itemSelectedBg: "#e4e4e7",
    itemSelectedColor: "#18181b",
    itemActiveBg: "#e4e4e7",
    itemHoverBg: "#f4f4f5",
    itemHoverColor: "#18181b",
  },
  Table: {
    headerBg: "#fafafa",
    headerSplitColor: "#e5e5e5",
    fontWeightStrong: 600,
  },
};
