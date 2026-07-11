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

import { Avatar, Tooltip } from "@wso2/oxygen-ui";
import type { JSX } from "react";

function getInitials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

export interface UserAvatarProps {
  name: string;
  /** Profile photo URL from Asgardeo. Falls back to initials if not provided. */
  src?: string | null;
  /** Avatar diameter in px. Defaults to 28. */
  size?: number;
}

export default function UserAvatar({ name, src, size = 28 }: UserAvatarProps): JSX.Element {
  return (
    <Tooltip title={name} arrow>
      <Avatar
        src={src ?? undefined}
        alt={name}
        sx={{ width: size, height: size, fontSize: size * 0.38, flexShrink: 0 }}
      >
        {!src && getInitials(name)}
      </Avatar>
    </Tooltip>
  );
}
