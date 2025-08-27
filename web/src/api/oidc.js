import { UserManager, WebStorageStateStore } from "oidc-client-ts";
import { hubApi } from "./index.js";

let userManager = null;
let oidcEnabled = false;

export async function initOidc() {
  if (userManager) return userManager; // 已初始化

  // 从后端获取配置
  const cfg = await hubApi.getAuthConfig();
  if (!cfg.oidc_enabled){
    return;
  }

  // 组装 oidc-client-ts 配置
  const oidcConfig = {
    authority: cfg.issuer,
    client_id: cfg.client_id,
    // Prefer configured callback, fallback to our router path
    redirect_uri: cfg.redirect_uri || (window.location.origin + "/oidc/callback"),
    post_logout_redirect_uri: cfg.post_logout_redirect_uri || window.location.origin,
    response_type: "code",
    scope: cfg.scope || "openid profile email",
    loadUserInfo: true,
    userStore: new WebStorageStateStore({ store: window.sessionStorage }),
  };

  userManager = new UserManager(oidcConfig);
  oidcEnabled = true;
  return userManager;
}

export function getUserManager() {
  if (!userManager) {
    throw new Error("OIDC 尚未初始化，请先调用 initOidc()");
  }
  return userManager;
}

export function isOidcEnabled() {
  return oidcEnabled;
}