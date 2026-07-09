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
  Card,
  CardActionArea,
  CardContent,
  Divider,
  IconButton,
  LinearProgress,
  ListItemIcon,
  Menu,
  MenuItem,
  Stack,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import { CalendarDays, MoreVertical, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useState } from "react";
import AuditStatusChip from "@modules/audit/components/AuditStatusChip";
import { formatDateRange } from "@modules/audit/utils/format";
import type { Audit } from "@modules/audit/types/audit";

interface AuditCardProps {
  audit: Audit;
  onClick: () => void;
  onDelete: () => void;
  canDelete?: boolean;
}

export default function AuditCard({ audit, onClick, onDelete, canDelete = false }: AuditCardProps): JSX.Element {
  const { controlCounts } = audit;
  const approvedPct =
    controlCounts.total > 0
      ? Math.round((controlCounts.approved / controlCounts.total) * 100)
      : 0;

  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);

  const handleMenuOpen = (e: React.MouseEvent<HTMLElement>) => {
    setMenuAnchor(e.currentTarget);
  };
  const handleDelete = () => {
    setMenuAnchor(null);
    onDelete();
  };

  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        position: "relative",
        transition: "box-shadow 0.2s",
        "&:hover": { boxShadow: 4 },
      }}
    >
      {/* Menu button — only visible to users with delete privilege */}
      {canDelete && (
        <>
          <Box sx={{ position: "absolute", top: 8, right: 8, zIndex: 1 }}>
            <IconButton size="small" aria-label="Audit actions" onClick={handleMenuOpen} sx={{ color: "text.secondary" }}>
              <MoreVertical size={16} />
            </IconButton>
          </Box>
          <Menu
            anchorEl={menuAnchor}
            open={Boolean(menuAnchor)}
            onClose={() => setMenuAnchor(null)}
          >
            <MenuItem onClick={handleDelete} sx={{ color: "error.main" }}>
              <ListItemIcon sx={{ color: "error.main" }}>
                <Trash2 size={16} />
              </ListItemIcon>
              Delete audit
            </MenuItem>
          </Menu>
        </>
      )}

      <CardActionArea
        onClick={onClick}
        sx={{ height: "100%", alignItems: "flex-start" }}
      >
        <CardContent sx={{ height: "100%", display: "flex", flexDirection: "column", gap: 1.5, p: 2.5 }}>
          {/* Top row: status chip — leave space on right for the absolute menu button */}
          <Box sx={{ display: "flex", alignItems: "center", pr: 3.5 }}>
            <AuditStatusChip status={audit.status} />
          </Box>

          {/* Audit name */}
          <Typography
            variant="h6"
            sx={{ fontWeight: 600, lineHeight: 1.3, mt: -0.5 }}
          >
            {audit.name}
          </Typography>

          {/* Framework · Product */}
          <Typography variant="body2" color="text.secondary">
            {audit.framework.name}
            {" · "}
            {audit.product.name}
          </Typography>

          {/* Period */}
          <Stack direction="row" spacing={0.75} alignItems="center">
            <CalendarDays size={14} style={{ color: "var(--mui-palette-text-secondary, #666)" }} />
            <Typography variant="caption" color="text.secondary">
              {formatDateRange(audit.periodStart, audit.periodEnd)}
            </Typography>
          </Stack>

          <Divider sx={{ mt: "auto" }} />

          {/* Progress */}
          <Box>
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                mb: 0.75,
              }}
            >
              <Typography variant="caption" color="text.secondary">
                {controlCounts.approved} / {controlCounts.total} approved
              </Typography>
              {controlCounts.overdue > 0 && (
                <Typography variant="caption" color="error.main" fontWeight={600}>
                  {controlCounts.overdue} overdue
                </Typography>
              )}
            </Box>
            <LinearProgress
              variant="determinate"
              value={approvedPct}
              color={controlCounts.overdue > 0 ? "warning" : "success"}
              sx={{ height: 6, borderRadius: 3 }}
            />
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}
