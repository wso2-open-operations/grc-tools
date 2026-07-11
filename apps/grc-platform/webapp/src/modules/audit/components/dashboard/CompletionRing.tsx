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

import { Box, CircularProgress, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";

interface CompletionRingProps {
  percent: number;
  /** Ring diameter in px. @default 92 */
  size?: number;
  /** Show the "complete" caption under the percentage. @default true */
  caption?: boolean;
}

// Determinate progress ring with a centered percentage label.
// Used by the dashboard hero band and audit cards.
export default function CompletionRing({ percent, size = 92, caption = true }: CompletionRingProps): JSX.Element {
  const value = Math.min(Math.max(percent, 0), 100);
  const small = size < 64;
  return (
    <Box sx={{ position: "relative", display: "inline-flex" }}>
      <CircularProgress
        variant="determinate"
        value={100}
        size={size}
        thickness={4}
        sx={{ color: "divider", position: "absolute" }}
      />
      <CircularProgress
        variant="determinate"
        value={value}
        size={size}
        thickness={4}
        sx={{ color: value >= 100 ? "#22C55E" : "primary.main", "& .MuiCircularProgress-circle": { strokeLinecap: "round" } }}
      />
      <Box sx={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>
        <Typography variant={small ? "caption" : "h6"} fontWeight={700} lineHeight={1}>
          {value.toFixed(0)}%
        </Typography>
        {caption && !small && (
          <Typography variant="caption" color="text.secondary" sx={{ fontSize: "0.6rem" }}>
            complete
          </Typography>
        )}
      </Box>
    </Box>
  );
}
