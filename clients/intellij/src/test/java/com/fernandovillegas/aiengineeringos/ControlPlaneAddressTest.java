package com.fernandovillegas.aiengineeringos;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

final class ControlPlaneAddressTest {
    @Test
    void acceptsExplicitLoopbackAddresses() {
        assertEquals("127.0.0.1:8765", ControlPlaneAddress.parse("127.0.0.1:8765").authority());
        assertEquals("127.12.1.9:9000", ControlPlaneAddress.parse("http://127.12.1.9:9000").authority());
        assertEquals("[::1]:8765", ControlPlaneAddress.parse("http://[::1]:8765").authority());
        assertTrue(ControlPlaneAddress.isLoopbackHost("localhost"));
    }

    @Test
    void rejectsRemoteOrAmbiguousAddresses() {
        assertThrows(IllegalArgumentException.class, () -> ControlPlaneAddress.parse("0.0.0.0:8765"));
        assertThrows(IllegalArgumentException.class, () -> ControlPlaneAddress.parse("192.168.1.5:8765"));
        assertThrows(IllegalArgumentException.class, () -> ControlPlaneAddress.parse("example.com:8765"));
        assertThrows(IllegalArgumentException.class, () -> ControlPlaneAddress.parse("http://127.0.0.1"));
        assertThrows(IllegalArgumentException.class, () -> ControlPlaneAddress.parse("file:///tmp/control-plane"));
    }

    @Test
    void buildsDashboardUriWithSelectedRun() {
        assertEquals(
                "http://127.0.0.1:8765/ui/?run=run+%2F+1",
                ControlPlaneAddress.parse("127.0.0.1:8765").dashboardUri("run / 1").toString()
        );
    }
}
