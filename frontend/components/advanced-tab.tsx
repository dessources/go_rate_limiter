import { TabsContent } from "@/components/ui/tabs";
import StressTest from "./stress-test";
import LiveMetrics from "./live-metrics";

import type { LiveMetricsProps } from "@/components/live-metrics";

export default function AdvancedTab(props: LiveMetricsProps & any) {
  return (
    <TabsContent value="advanced" className="mt-6 space-y-8">
      <LiveMetrics metrics={props.metrics} setMetrics={props.setMetrics} />
      <StressTest />
    </TabsContent>
  );
}
