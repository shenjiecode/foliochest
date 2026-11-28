import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { setNonce } from "get-nonce";
import { TooltipProvider } from "@/components/ui/tooltip";
import { App } from "./app";
import { SessionClient } from "./lib/client";
const root = document.getElementById("app")!;
const nonce = document.querySelector<HTMLMetaElement>(
  'meta[name="style-nonce"]',
)?.content;
if (!nonce || !/^[a-f0-9]{48}$/.test(nonce)) {
  root.textContent = "页面加载异常，请重新打开资料匣。";
} else {
  setNonce(nonce);
  const client = new SessionClient();
  createRoot(root).render(
    <StrictMode>
      <TooltipProvider delayDuration={350}>
        <App client={client} />
      </TooltipProvider>
    </StrictMode>,
  );
}
