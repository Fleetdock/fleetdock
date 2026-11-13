"use client";

import { ChannelsSection } from "@/components/notifications/channels";
import { RulesSection } from "@/components/notifications/rules";
import { PageHeader } from "@/components/ui";
import { useCan } from "@/lib/hooks";

export default function NotificationsPage() {
  const can = useCan();
  const canWrite = can("notification:write");

  return (
    <div>
      <PageHeader
        title="Notifications"
        description="Where Fleetdock sends alerts (email, Slack, webhooks, PagerDuty) and which events trigger them."
      />
      <ChannelsSection canWrite={canWrite} />
      <RulesSection canWrite={canWrite} />
    </div>
  );
}
