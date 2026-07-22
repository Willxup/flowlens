/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_FLOWLENS_MODE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
