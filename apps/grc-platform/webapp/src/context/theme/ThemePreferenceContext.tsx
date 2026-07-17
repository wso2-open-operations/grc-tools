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

import { useState, type JSX } from "react";
import type { OxygenTheme } from "@wso2/oxygen-ui/styles/OxygenThemeBase";
import { themes, THEME_OPTIONS, isThemeKey, themeConfig } from "@config/themeConfig";
import { ThemePreferenceContext } from "./useThemePreference";

const STORAGE_KEY = "grc-platform-theme";

function resolveInitialKey(): string {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored && isThemeKey(stored)) return stored;
  const fromConfig = window.config?.GRC_PLATFORM_THEME;
  if (fromConfig && isThemeKey(fromConfig)) return fromConfig;
  return "acrylicOrange";
}

export function ThemePreferenceProvider({ children }: { children: React.ReactNode }): JSX.Element {
  const [themeKey, setThemeKeyState] = useState<string>(resolveInitialKey);

  const setThemeKey = (key: string): void => {
    if (!isThemeKey(key)) return;
    localStorage.setItem(STORAGE_KEY, key);
    setThemeKeyState(key);
  };

  const theme = (themes[themeKey] ?? themeConfig) as OxygenTheme;

  return (
    <ThemePreferenceContext.Provider value={{ themeKey, theme, options: THEME_OPTIONS, setThemeKey }}>
      {children}
    </ThemePreferenceContext.Provider>
  );
}
