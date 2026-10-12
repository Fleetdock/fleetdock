"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function Home() {
  const router = useRouter();
  useEffect(() => {
    // The session cookie is httpOnly, so it cannot be checked here: the
    // dashboard shell verifies it and sends signed-out users to /login.
    router.replace("/dashboard");
  }, [router]);
  return null;
}
