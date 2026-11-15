import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";

const eslintConfig = defineConfig([
  ...nextVitals,
  globalIgnores([".next/**", "out/**", "build/**", "next-env.d.ts"]),
  {
    rules: {
      // Existing modal/form reset patterns predate this React 19 rule.
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/exhaustive-deps": "warn",
      // Both exist only in secure contexts; Fleetdock is often served over
      // plain HTTP on a LAN address.
      "no-restricted-properties": [
        "error",
        { object: "navigator", property: "clipboard", message: "Use copyText or useCopy, which work over plain HTTP." },
        { object: "crypto", property: "randomUUID", message: "Use randomUUID from @/lib/uuid, which works over plain HTTP." },
      ],
    },
  },
  {
    files: ["src/components/ui/base.tsx", "src/lib/uuid.ts", "src/lib/uuid.test.ts"],
    rules: { "no-restricted-properties": "off" },
  },
]);

export default eslintConfig;
