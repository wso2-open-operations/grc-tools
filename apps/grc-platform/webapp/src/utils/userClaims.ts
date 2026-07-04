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

export interface UserInfo {
  fullName: string;
  email: string;
  avatarUrl: string;
  orgName: string;
  orgHandle: string;
  groups: string[];
}

// Maps raw ID token claims to a typed UserInfo object.
export function resolveUserInfo(
  claims: Record<string, unknown> | null
): UserInfo {
  if (!claims) {
    return {
      fullName: "",
      email: "",
      avatarUrl: "",
      orgName: "",
      orgHandle: "",
      groups: [],
    };
  }

  const given = (claims.given_name as string) ?? "";
  const family = (claims.family_name as string) ?? "";
  const fullName =
    [given, family].filter(Boolean).join(" ") ||
    (claims.username as string) ||
    (claims.sub as string) ||
    "";

  return {
    fullName,
    email: (claims.email as string) ?? "",
    avatarUrl: (claims.picture as string) ?? "",
    orgName: (claims.org_name as string) ?? "",
    orgHandle: (claims.org_id as string) ?? "",
    groups: Array.isArray(claims.groups) ? (claims.groups as string[]) : [],
  };
}

// Returns up to two initials from a display name (e.g. "Jane Doe" → "JD").
export function initialsOf(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join("");
}
