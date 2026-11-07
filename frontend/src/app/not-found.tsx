import { Compass } from "lucide-react";

import { Logo } from "@/components/logo";
import { ButtonLink, EmptyState } from "@/components/ui";

// Any address no route matches (outside the dashboard's own not-found, which
// keeps the sidebar). Renders inside the root layout, so theme and fonts apply.
export default function NotFound() {
  return (
    <main className="standalone-page">
      <div className="flex items-center gap-2 mb-6">
        <Logo size={28} />
        <span className="font-semibold">Fleetdock</span>
      </div>
      <EmptyState
        icon={<Compass size={22} />}
        title="Page not found"
        hint="The link may be old, or the page has moved."
        action={
          <ButtonLink href="/dashboard" variant="primary">
            Go to overview
          </ButtonLink>
        }
      />
    </main>
  );
}
