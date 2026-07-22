import { lazy, Suspense } from "react";
import type { FlowLensDataSource } from "../api/source";
import { LoginPage } from "../features/auth/LoginPage";

const DashboardPage = lazy(async () => {
  const module = await import("../features/dashboard/DashboardPage");
  return { default: module.DashboardPage };
});

export function App({ source }: { source: FlowLensDataSource }) {
  const login = !source.demo && window.location.pathname === "/login";
  const navigate = (path: string) => window.location.replace(path);
  if (login)
    return <LoginPage source={source} onAuthenticated={() => navigate("/")} />;
  return (
    <Suspense
      fallback={<main className="loading-screen">正在加载 FlowLens…</main>}
    >
      <DashboardPage
        source={source}
        onUnauthorized={() => navigate("/login")}
      />
    </Suspense>
  );
}
