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
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
  Box,
} from "@wso2/oxygen-ui";
import { Palette, Check } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useState } from "react";
import { useThemePreference } from "@context/theme/ThemePreferenceContext";
import { isThemeKey } from "@config/themeConfig";

export default function ThemeSelect(): JSX.Element {
  const { themeKey, setThemeKey, options } = useThemePreference();
  const [anchor, setAnchor] = useState<null | HTMLElement>(null);

  const handleOpen = (e: React.MouseEvent<HTMLElement>): void => {
    setAnchor(e.currentTarget);
  };

  const handleClose = (): void => {
    setAnchor(null);
  };

  const handleSelect = (key: string): void => {
    if (isThemeKey(key)) setThemeKey(key);
    handleClose();
  };

  return (
    <>
      <Tooltip title="Theme">
        <IconButton onClick={handleOpen} size="small" aria-label="Select theme">
          <Palette size={20} />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchor}
        open={Boolean(anchor)}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        {options.map((o) => (
          <MenuItem key={o.key} onClick={() => handleSelect(o.key)} selected={o.key === themeKey}
            sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 16, display: "flex", alignItems: "center", flexShrink: 0 }}>
              {o.key === themeKey && <Check size={16} />}
            </Box>
            {o.label}
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}
