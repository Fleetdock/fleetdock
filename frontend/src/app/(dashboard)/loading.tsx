import { PageSkeleton } from "@/components/ui";

// Shown while a page's code loads on navigation; pages show their own
// skeletons while their data loads.
export default function DashboardLoading() {
  return <PageSkeleton />;
}
