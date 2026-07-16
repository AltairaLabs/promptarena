import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { AtlasSessionPreview } from "./components/_preview/AtlasSessionPreview";

// Throwaway: load /#atlas to preview the real Atlas SessionReview over a run.
const isAtlasPreview = window.location.hash === "#atlas";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {isAtlasPreview ? <AtlasSessionPreview /> : <App />}
  </StrictMode>
);
