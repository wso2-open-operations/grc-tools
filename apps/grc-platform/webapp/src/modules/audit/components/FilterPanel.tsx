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
  Box,
  Checkbox,
  FormControl,
  IconButton,
  InputAdornment,
  ListItemText,
  MenuItem,
  Select,
  TextField,
} from "@wso2/oxygen-ui";
import { Search, X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";

export interface FilterField {
  id: string;
  label: string;
  options: { label: string; value: string }[];
}

interface FilterPanelProps {
  /** Dropdown filter fields rendered alongside the search box. */
  fields?: FilterField[];
  /** Currently active filter selections, keyed by FilterField.id. */
  values?: Record<string, string[]>;
  onChange?: (values: Record<string, string[]>) => void;
  search: string;
  onSearchChange: (search: string) => void;
  searchPlaceholder?: string;
}

export default function FilterPanel({
  fields,
  values,
  onChange,
  search,
  onSearchChange,
  searchPlaceholder = "Search...",
}: FilterPanelProps): JSX.Element {
  return (
    <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "center" }}>
      {/* Search */}
      <TextField
        size="small"
        placeholder={searchPlaceholder}
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        sx={{ flex: "1 1 240px", minWidth: 200 }}
        slotProps={{
          input: {
            startAdornment: (
              <InputAdornment position="start">
                <Search size={16} />
              </InputAdornment>
            ),
            endAdornment: search ? (
              <InputAdornment position="end">
                <IconButton
                  size="small"
                  edge="end"
                  aria-label="Clear search"
                  onClick={() => onSearchChange("")}
                >
                  <X size={14} />
                </IconButton>
              </InputAdornment>
            ) : null,
          },
        }}
      />

      {/* Dropdown filters */}
      {fields?.map((field) => {
        const selected = values?.[field.id] ?? [];
        return (
          <FormControl key={field.id} size="small" sx={{ minWidth: 130 }}>
            <Select
              multiple
              displayEmpty
              value={selected}
              onChange={(e) => {
                const v = e.target.value as string[];
                onChange?.({ ...values, [field.id]: v });
              }}
              renderValue={(sel) => {
                const s = sel as string[];
                if (s.length === 0) return <em style={{ color: "#9e9e9e" }}>{field.label}</em>;
                return `${field.label} (${s.length})`;
              }}
              sx={{ fontSize: "0.875rem" }}
            >
              {field.options.map((opt) => (
                <MenuItem key={opt.value} value={opt.value} dense>
                  <Checkbox
                    checked={selected.includes(opt.value)}
                    size="small"
                    sx={{ py: 0, pl: 0, pr: 0.5 }}
                  />
                  <ListItemText
                    primary={opt.label}
                    slotProps={{ primary: { style: { fontSize: "0.875rem" } } }}
                  />
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        );
      })}
    </Box>
  );
}
