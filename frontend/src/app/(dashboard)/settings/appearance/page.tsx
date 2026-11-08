"use client";

import { Monitor, Moon, Palette, Sun } from "lucide-react";

import { Card, CardBody, CardHeader, PageHeader, SegmentedControl, Switch, type SegmentOption } from "@/components/ui";
import { useAppearance, type StylePref, type ThemePref } from "@/lib/appearance";

const THEMES: SegmentOption<ThemePref>[] = [
  { value: "light", label: <><Sun size={14} aria-hidden /> Light</> },
  { value: "dark", label: <><Moon size={14} aria-hidden /> Dark</> },
  { value: "system", label: <><Monitor size={14} aria-hidden /> System</> },
];

const STYLES: SegmentOption<StylePref>[] = [
  { value: "classic", label: "Classic" },
  { value: "neutral", label: "Neutral" },
];

export default function AppearancePage() {
  const { theme, style, borderless, setTheme, setStyle, setBorderless } = useAppearance();

  return (
    <div className="max-w-xl">
      <PageHeader title="Appearance" description="Style, theme and borders. Saved in this browser." />
      <Card>
        <CardHeader icon={<Palette size={15} aria-hidden className="text-muted" />} title="Display" />
        <CardBody className="stack">
          <div className="stack-sm">
            <span className="font-medium">Style</span>
            <SegmentedControl label="Style" options={STYLES} value={style} onChange={setStyle} />
            <span className="text-muted text-sm">
              Classic is the standard teal look. Neutral is a compact, gray design.
            </span>
          </div>
          <div className="stack-sm">
            <span className="font-medium">Theme</span>
            <SegmentedControl label="Theme" options={THEMES} value={theme} onChange={setTheme} />
            <span className="text-muted text-sm">System follows your operating system setting.</span>
          </div>
          <div className="stack-sm">
            <Switch label="Borderless UI" checked={borderless} onCheckedChange={setBorderless} />
            <span className="text-muted text-sm">
              Hides most borders (cards, tables, top bar, sidebar). Form fields and the data browser&apos;s tabs keep
              theirs.
            </span>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
