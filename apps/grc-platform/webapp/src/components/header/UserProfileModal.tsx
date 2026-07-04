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
  Avatar,
  Box,
  Button,
  Chip,
  Dialog,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import { type JSX } from "react";
import { useIdTokenClaims } from "@hooks/useIdTokenClaims";
import { initialsOf, resolveUserInfo } from "@utils/userClaims";

// Show at most this many group chips before collapsing into "+N more".
const GROUPS_PREVIEW_LIMIT = 8;

interface UserProfileModalProps {
  open: boolean;
  onClose: () => void;
}

// Read-only profile dialog: picture, name, email, organization, and groups.
// All values come from the ID token claims (no backend call).
export default function UserProfileModal({
  open,
  onClose,
}: UserProfileModalProps): JSX.Element {
  const claims = useIdTokenClaims();
  const info = resolveUserInfo(claims);
  const initials = initialsOf(info.fullName);

  const visibleGroups = info.groups.slice(0, GROUPS_PREVIEW_LIMIT);
  const hiddenGroupCount = info.groups.length - visibleGroups.length;

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          <Typography variant="h6">Profile</Typography>
          <IconButton onClick={onClose} size="small" aria-label="Close">
            <X size={18} />
          </IconButton>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Avatar
            src={info.avatarUrl}
            imgProps={{ referrerPolicy: "no-referrer" }}
            sx={{ width: 64, height: 64, fontSize: 24 }}
          >
            {initials}
          </Avatar>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="subtitle1" noWrap>
              {info.fullName}
            </Typography>
            <Typography variant="body2" color="text.secondary" noWrap>
              {info.email || "—"}
            </Typography>
            {info.orgName && (
              <Typography variant="caption" color="text.secondary">
                {info.orgName}
                {info.orgHandle && info.orgHandle !== info.orgName
                  ? ` (${info.orgHandle})`
                  : ""}
              </Typography>
            )}
          </Box>
        </Box>

        {info.groups.length > 0 && (
          <Box>
            <Typography variant="caption" color="text.secondary">
              Groups
            </Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
              {visibleGroups.map((g) => (
                <Chip key={g} size="small" label={g} variant="outlined" />
              ))}
              {hiddenGroupCount > 0 && (
                <Chip
                  size="small"
                  label={`+${hiddenGroupCount} more`}
                  variant="outlined"
                />
              )}
            </Box>
          </Box>
        )}

        <Stack direction="row" justifyContent="flex-end">
          <Button variant="contained" onClick={onClose}>
            Close
          </Button>
        </Stack>
      </Box>
    </Dialog>
  );
}
