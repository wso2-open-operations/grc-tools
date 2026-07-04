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

import { useAsgardeo } from "@asgardeo/react";
import { useEffect, useState } from "react";

// Returns the decoded ID token claims object, or null while loading / in mock mode.
export function useIdTokenClaims(): Record<string, unknown> | null {
  const { getDecodedIdToken, isSignedIn } = useAsgardeo();
  const [claims, setClaims] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    if (!isSignedIn) {
      setClaims(null);
      return;
    }
    getDecodedIdToken()
      .then((token) => setClaims(token as Record<string, unknown>))
      .catch(() => setClaims(null));
  }, [isSignedIn, getDecodedIdToken]);

  return claims;
}
