import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AxiosError } from "axios";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import Alert from "@mui/material/Alert";
import { PlusIcon, TrashIcon, CircleCheckFilledIcon } from "@oxygen-ui/react-icons";
import { evidenceApi } from "../api/client";
import ControlPicker from "../components/ControlPicker";
import ProductPicker from "../components/ProductPicker";
import FrameworkPicker from "../components/FrameworkPicker";
import { validateSubmissionFiles } from "../utils/validateSubmissionFiles";

export default function SubmitEvidence() {
  const queryClient = useQueryClient();
  const [productId, setProductId] = useState<number | "">("");
  const [frameworkId, setFrameworkId] = useState<number | "">("");
  const [controlId, setControlId] = useState<number | "">("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [success, setSuccess] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: evidenceApi.create,
    onSuccess: () => {
      setSuccess(true);
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      setTitle("");
      setDescription("");
      setFiles([]);
      setProductId("");
      setFrameworkId("");
      setControlId("");
    },
  });

  const uploadError = mutation.isError
    ? (mutation.error as AxiosError<{ detail?: string }>)?.response?.data?.detail ||
      "Upload failed. Please check the files and try again."
    : null;

  const handleFilesChosen = (e: React.ChangeEvent<HTMLInputElement>) => {
    const chosen = Array.from(e.target.files ?? []);
    // Appended, not replaced, so clicking to add more keeps what was already
    // chosen. Clearing the input's own value lets picking the same file
    // again after removing it register as a change.
    setFiles((prev) => [...prev, ...chosen]);
    e.target.value = "";
  };

  const handleRemoveFile = (index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // The server holds the real limits (also 4 files, also 20 MB); this is
    // only here so a doomed upload never starts.
    const filesCheck = validateSubmissionFiles(files);
    if (!filesCheck.valid) {
      setValidationError(filesCheck.message);
      return;
    }
    if (!controlId) {
      setValidationError("Please select a control.");
      return;
    }
    setValidationError(null);
    setSuccess(false);
    const formData = new FormData();
    formData.append("title", title);
    formData.append("description", description);
    formData.append("control_id", String(controlId));
    for (const file of files) {
      formData.append("file", file);
    }
    mutation.mutate(formData);
  };

  return (
    <Box sx={{ maxWidth: 640, mx: "auto" }}>
      <Box sx={{ textAlign: "center", mb: 4 }}>
        <Typography variant="h4" gutterBottom>
          Submit Evidence
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Upload up to 4 files and link them to a compliance control.
        </Typography>
      </Box>

      {success && (
        <Alert
          severity="success"
          icon={<CircleCheckFilledIcon size={18} />}
          sx={{ mb: 3 }}
        >
          Evidence submitted successfully.
        </Alert>
      )}

      {uploadError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {uploadError}
        </Alert>
      )}

      {validationError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {validationError}
        </Alert>
      )}

      <Paper variant="outlined" sx={{ p: { xs: 3, sm: 4 } }}>
        <Box component="form" onSubmit={handleSubmit}>
          <Stack spacing={2.5}>
            <ProductPicker
              value={productId}
              onChange={(id) => {
                setProductId(id);
                setFrameworkId("");
                setControlId("");
              }}
              required
              helperText="Pick the product this evidence belongs to."
            />

            <FrameworkPicker
              productId={productId}
              value={frameworkId}
              onChange={(id) => {
                setFrameworkId(id);
                setControlId("");
              }}
              required
            />

            <ControlPicker
              frameworkId={frameworkId}
              controlId={controlId}
              onControlChange={(id) => setControlId(id)}
              required
            />

            <TextField
              label="Title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Evidence title"
              required
              fullWidth
            />

            <TextField
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
              multiline
              rows={3}
              fullWidth
            />

            <Box>
              <Typography variant="caption" color="text.secondary" display="block" mb={0.75} fontWeight={600}>
                FILES *
              </Typography>
              <Button
                component="label"
                variant="outlined"
                fullWidth
                startIcon={<PlusIcon size={18} />}
                sx={{
                  py: 1.75,
                  borderStyle: "dashed",
                  borderColor: "divider",
                  color: files.length > 0 ? "text.primary" : "text.secondary",
                  justifyContent: "flex-start",
                  px: 2,
                  "&:hover": { borderStyle: "dashed", borderColor: "primary.main", backgroundColor: "rgba(255,115,0,0.04)" },
                }}
              >
                {files.length > 0 ? "Add more files" : "Click to select files"}
                <input
                  type="file"
                  hidden
                  multiple
                  onChange={handleFilesChosen}
                />
              </Button>

              {files.length > 0 && (
                <Stack spacing={0.5} sx={{ mt: 1.5 }}>
                  {files.map((f, index) => (
                    <Stack
                      key={`${f.name}-${index}`}
                      direction="row"
                      alignItems="center"
                      justifyContent="space-between"
                      sx={{
                        px: 1.5,
                        py: 0.75,
                        borderRadius: 1,
                        backgroundColor: "action.hover",
                      }}
                    >
                      <Typography variant="body2" noWrap sx={{ mr: 1 }}>
                        {f.name}
                      </Typography>
                      <IconButton
                        size="small"
                        aria-label={`Remove ${f.name}`}
                        onClick={() => handleRemoveFile(index)}
                      >
                        <TrashIcon size={14} />
                      </IconButton>
                    </Stack>
                  ))}
                </Stack>
              )}
            </Box>

            <Button
              type="submit"
              variant="contained"
              disabled={mutation.isPending}
              size="large"
              sx={{ mt: 1, py: 1.25 }}
            >
              {mutation.isPending ? "Uploading..." : "Submit Evidence"}
            </Button>
          </Stack>
        </Box>
      </Paper>
    </Box>
  );
}
