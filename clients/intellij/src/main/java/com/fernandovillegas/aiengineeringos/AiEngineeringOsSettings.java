package com.fernandovillegas.aiengineeringos;

import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.components.PersistentStateComponent;
import com.intellij.openapi.components.Service;
import com.intellij.openapi.components.State;
import com.intellij.openapi.components.Storage;
import org.jetbrains.annotations.NotNull;
import org.jetbrains.annotations.Nullable;

@Service(Service.Level.APP)
@State(name = "AiEngineeringOsSettings", storages = @Storage("ai-engineering-os.xml"))
public final class AiEngineeringOsSettings implements PersistentStateComponent<AiEngineeringOsSettings.SettingsState> {
    public static final class SettingsState {
        public String binaryPath = "ai";
        public String controlPlaneAddress = ControlPlaneAddress.DEFAULT;
    }

    private SettingsState state = new SettingsState();

    public static @NotNull AiEngineeringOsSettings getInstance() {
        return ApplicationManager.getApplication().getService(AiEngineeringOsSettings.class);
    }

    @Override
    public @Nullable SettingsState getState() {
        return state;
    }

    @Override
    public void loadState(@NotNull SettingsState state) {
        this.state = state;
    }

    public @NotNull String binaryPath() {
        String value = state.binaryPath == null ? "" : state.binaryPath.trim();
        return value.isEmpty() ? "ai" : value;
    }

    public @NotNull String controlPlaneAddress() {
        String value = state.controlPlaneAddress == null ? "" : state.controlPlaneAddress.trim();
        return value.isEmpty() ? ControlPlaneAddress.DEFAULT : value;
    }

    public void update(String binaryPath, String address) {
        state.binaryPath = binaryPath == null || binaryPath.isBlank() ? "ai" : binaryPath.trim();
        state.controlPlaneAddress = ControlPlaneAddress.parse(address).authority();
    }
}
