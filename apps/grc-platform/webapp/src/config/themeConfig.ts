// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import {
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
  ChoreoTheme,
  HighContrastTheme,
  ClassicTheme,
} from "@wso2/oxygen-ui";
import type { OxygenTheme } from "@wso2/oxygen-ui/styles/Themes/OxygenThemeBase";

export const themes: Record<string, OxygenTheme> = {
  acrylicOrange: AcrylicOrangeTheme,
  acrylicPurple: AcrylicPurpleTheme,
  choreo: ChoreoTheme,
  highContrast: HighContrastTheme,
  classic: ClassicTheme,
};

export const THEME_OPTIONS: { key: string; label: string }[] = [
  { key: "acrylicOrange", label: "Acrylic Orange" },
  { key: "acrylicPurple", label: "Acrylic Purple" },
  { key: "choreo", label: "Choreo" },
  { key: "highContrast", label: "High Contrast" },
  { key: "classic", label: "Classic" },
];

export function isThemeKey(key: string): key is keyof typeof themes {
  return key in themes;
}

export const themeConfig =
  themes[window.config?.GRC_PLATFORM_THEME || "acrylicOrange"] ||
  AcrylicOrangeTheme;
