import { RouteLoading } from "@/components/page-transition";
import { PageSkeleton } from "@/components/ui";

// Shown while a page's code loads on navigation; pages show their own
// skeletons while their data loads. RouteLoading fades the page in over it.
export default function DashboardLoading() {
  return (
    <RouteLoading>
      <PageSkeleton />
    </RouteLoading>
  );
}
