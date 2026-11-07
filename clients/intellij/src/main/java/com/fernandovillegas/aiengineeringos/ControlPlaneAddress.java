package com.fernandovillegas.aiengineeringos;

import org.jetbrains.annotations.NotNull;

import java.net.URI;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.Locale;

public final class ControlPlaneAddress {
    public static final String DEFAULT = "127.0.0.1:8765";

    private final URI base;

    private ControlPlaneAddress(URI base) {
        this.base = base;
    }

    public static @NotNull ControlPlaneAddress parse(String raw) {
        String value = raw == null ? "" : raw.trim();
        if (value.isEmpty()) {
            value = DEFAULT;
        }
        if (!value.contains("://")) {
            value = "http://" + value;
        }

        final URI parsed;
        try {
            parsed = URI.create(value);
        } catch (IllegalArgumentException error) {
            throw new IllegalArgumentException("Control Plane address must be host:port or an http(s) URL.", error);
        }

        String scheme = parsed.getScheme() == null ? "" : parsed.getScheme().toLowerCase(Locale.ROOT);
        if (!scheme.equals("http") && !scheme.equals("https")) {
            throw new IllegalArgumentException("Control Plane URL must use http or https.");
        }
        if (parsed.getUserInfo() != null) {
            throw new IllegalArgumentException("Control Plane URL must not contain user info.");
        }
        String host = stripBrackets(parsed.getHost());
        if (host == null || !isLoopbackHost(host)) {
            throw new IllegalArgumentException("Control Plane address must use localhost or a loopback IP.");
        }
        if (parsed.getPort() < 1 || parsed.getPort() > 65535) {
            throw new IllegalArgumentException("Control Plane address must include an explicit port.");
        }
        if (parsed.getQuery() != null || parsed.getFragment() != null) {
            throw new IllegalArgumentException("Control Plane base address must not contain query or fragment.");
        }

        try {
            URI normalized = new URI(
                    scheme,
                    null,
                    host,
                    parsed.getPort(),
                    "/",
                    null,
                    null
            );
            return new ControlPlaneAddress(normalized);
        } catch (Exception error) {
            throw new IllegalArgumentException("Unable to normalize Control Plane address.", error);
        }
    }

    public static boolean isLoopbackHost(String hostname) {
        if (hostname == null) return false;
        String host = stripBrackets(hostname).trim().toLowerCase(Locale.ROOT);
        if (host.equals("localhost") || host.equals("::1") || host.equals("0:0:0:0:0:0:0:1")) {
            return true;
        }
        String[] parts = host.split("\\.");
        if (parts.length == 4) {
            try {
                int first = Integer.parseInt(parts[0]);
                for (String part : parts) {
                    int value = Integer.parseInt(part);
                    if (value < 0 || value > 255) return false;
                }
                return first == 127;
            } catch (NumberFormatException ignored) {
                return false;
            }
        }
        return false;
    }

    public @NotNull String authority() {
        String host = stripBrackets(base.getHost());
        if (host.contains(":")) {
            host = "[" + host + "]";
        }
        return host + ":" + base.getPort();
    }

    private static String stripBrackets(String host) {
        if (host == null) return null;
        String value = host.trim();
        if (value.startsWith("[") && value.endsWith("]") && value.length() > 2) {
            return value.substring(1, value.length() - 1);
        }
        return value;
    }

    public @NotNull URI dashboardUri() {
        return base.resolve("/ui/");
    }

    public @NotNull URI dashboardUri(String runId) {
        if (runId == null || runId.isBlank()) {
            return dashboardUri();
        }
        String encoded = URLEncoder.encode(runId, StandardCharsets.UTF_8);
        return URI.create(dashboardUri() + "?run=" + encoded);
    }

    @Override
    public String toString() {
        return base.toString();
    }
}
