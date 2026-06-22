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

import "@config/portalConfig";

export interface AuthConfig {
  baseUrl: string;
  clientId: string;
  signInRedirectURL: string;
  signOutRedirectURL: string;
}

const getAuthConfig = (): AuthConfig => {
  const config = window.config;
  const defaultRedirectURL = new URL(
    import.meta.env.BASE_URL,
    window.location.origin,
  ).toString();
  const isMockAuth = config?.GRC_PLATFORM_MOCK_AUTH === true;
  const baseUrl = config?.GRC_PLATFORM_AUTH_BASE_URL || "";
  const clientId = config?.GRC_PLATFORM_AUTH_CLIENT_ID || "";

  if (!isMockAuth && (!baseUrl || !clientId)) {
    console.error(
      "[AuthConfig] Missing required auth configuration. Set GRC_PLATFORM_AUTH_BASE_URL and GRC_PLATFORM_AUTH_CLIENT_ID in public/config.js or enable GRC_PLATFORM_MOCK_AUTH for local development."
    );
  }
  return {
    baseUrl,
    clientId,
    signInRedirectURL:
      config?.GRC_PLATFORM_AUTH_SIGN_IN_REDIRECT_URL || defaultRedirectURL,
    signOutRedirectURL:
      config?.GRC_PLATFORM_AUTH_SIGN_OUT_REDIRECT_URL || defaultRedirectURL,
  };
};

export const authConfig = getAuthConfig();
