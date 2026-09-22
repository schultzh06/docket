import React, { useEffect, useState } from "react";
import { render, Box, Text, useApp } from "ink";
import { createClient, ConnectError, Code, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { DocketService, type GetStatusResponse } from "./gen/docket/v1/docket_pb.js";

const baseUrl = process.env.DOCKET_URL ?? "http://127.0.0.1:8080";
const token = process.env.DOCKET_TOKEN;
if (!token) {
  console.error("DOCKET_TOKEN not set");
  process.exit(1);
}

// Runs on every request: the one place the token gets attached.
const auth: Interceptor = (next) => async (req) => {
  req.header.set("Authorization", `Bearer ${token}`);
  return next(req);
};

const transport = createConnectTransport({
  baseUrl,
  httpVersion: "1.1",
  interceptors: [auth],
});
const client = createClient(DocketService, transport);

type State =
  | { kind: "loading" }
  | { kind: "ok"; res: GetStatusResponse }
  | { kind: "error"; msg: string };

function App() {
  const { exit } = useApp();
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    client
      .getStatus({}, { timeoutMs: 5000 })
      .then((res) => setState({ kind: "ok", res }))
      .catch((err: unknown) => {
        const e = ConnectError.from(err);
        const msg =
          e.code === Code.Unauthenticated ? "bad token"
          : e.code === Code.Unavailable ? `daemon unreachable at ${baseUrl}`
          : e.code === Code.DeadlineExceeded ? "timed out"
          : e.message;
        setState({ kind: "error", msg });
      });
  }, []);

  // One-shot for now: exit once there's a result.
  useEffect(() => {
    if (state.kind !== "loading") exit();
  }, [state, exit]);

  if (state.kind === "loading") return <Text dimColor>connecting…</Text>;
  if (state.kind === "error") return <Text color="red">✗ {state.msg}</Text>;

  const t = state.res.serverTime ? timestampDate(state.res.serverTime) : undefined;
  return (
    <Box flexDirection="column">
      <Text color="green">✓ docket {state.res.version}</Text>
      <Text>server time: {t?.toLocaleString() ?? "unknown"}</Text>
    </Box>
  );
}

render(<App />);