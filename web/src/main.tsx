import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import { createDataSource } from "./api/create-source";
import "./styles/tokens.css";
import "./styles/layout.css";
import "./styles/components.css";

const root = document.getElementById("root");
if (root === null) throw new Error("FlowLens root is unavailable");

createRoot(root).render(
  <StrictMode>
    <App source={createDataSource()} />
  </StrictMode>,
);
