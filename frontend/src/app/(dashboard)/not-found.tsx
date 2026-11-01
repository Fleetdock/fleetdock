import { Compass } from "lucide-react";

import { ButtonLink, EmptyState } from "@/components/ui";

export default function DashboardNotFound() {
  return (
    <EmptyState
      icon={<Compass size={22} />}
      title="Page not found"
      hint="The link may be old, or the page has moved. Use search (Ctrl K) to find what you were looking for."
      action={
        <ButtonLink href="/dashboard" variant="primary">
          Go to overview
        </ButtonLink>
      }
    />
  );
}
