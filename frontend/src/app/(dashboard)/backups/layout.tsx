import type { ReactNode } from "react";

import { SectionLayout } from "@/components/section-tabs";

export default function BackupsLayout({ children }: { children: ReactNode }) {
  return <SectionLayout section="Backups">{children}</SectionLayout>;
}
