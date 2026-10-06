import type { ReactNode } from "react";

import { SectionLayout } from "@/components/section-tabs";

export default function AccessLayout({ children }: { children: ReactNode }) {
  return <SectionLayout section="Access">{children}</SectionLayout>;
}
