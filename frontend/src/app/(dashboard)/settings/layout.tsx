import type { ReactNode } from "react";

import { SectionLayout } from "@/components/section-tabs";

export default function SettingsLayout({ children }: { children: ReactNode }) {
  return <SectionLayout section="Settings">{children}</SectionLayout>;
}
