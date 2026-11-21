import {defineConfig,globalIgnores} from "eslint/config";
import js from "@eslint/js";
import ts from "typescript-eslint";
import next from "@next/eslint-plugin-next";
import hooks from "eslint-plugin-react-hooks";
import globals from "globals";
export default defineConfig([
 globalIgnores([".next/**","out/**","next-env.d.ts","playwright-report/**","test-results/**"]),
 js.configs.recommended,
 ts.configs.recommended,
 {languageOptions:{globals:{...globals.browser,...globals.node}}},
 {files:["**/*.tsx"],plugins:{"@next/next":next,"react-hooks":hooks},rules:{...next.configs.recommended.rules,...next.configs["core-web-vitals"].rules,...hooks.configs.recommended.rules}},
]);
