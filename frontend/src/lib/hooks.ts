"use client";

// The data hooks live in lib/data/<area>.ts; this barrel keeps the old
// "@/lib/hooks" import path working.
export * from "./data/core";
export * from "./data/servers";
export * from "./data/instances";
export * from "./data/databases";
export * from "./data/operations";
export * from "./data/backups";
export * from "./data/access";
export * from "./data/notifications";
export * from "./data/overview";
