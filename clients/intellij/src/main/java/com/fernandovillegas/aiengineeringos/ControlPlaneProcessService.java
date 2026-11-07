package com.fernandovillegas.aiengineeringos;

import com.intellij.execution.ExecutionException;
import com.intellij.execution.configurations.GeneralCommandLine;
import com.intellij.execution.process.OSProcessHandler;
import com.intellij.openapi.Disposable;
import com.intellij.openapi.components.Service;
import com.intellij.openapi.project.Project;
import org.jetbrains.annotations.NotNull;

import java.nio.charset.StandardCharsets;

@Service(Service.Level.PROJECT)
public final class ControlPlaneProcessService implements Disposable {
    private final Project project;
    private OSProcessHandler handler;

    public ControlPlaneProcessService(Project project) {
        this.project = project;
    }

    public synchronized boolean start() throws ExecutionException {
        if (handler != null && !handler.isProcessTerminated()) {
            return false;
        }

        AiEngineeringOsSettings settings = AiEngineeringOsSettings.getInstance();
        ControlPlaneAddress address = ControlPlaneAddress.parse(settings.controlPlaneAddress());

        GeneralCommandLine command = new GeneralCommandLine(
                settings.binaryPath(),
                "run",
                "serve",
                "--addr",
                address.authority()
        ).withCharset(StandardCharsets.UTF_8);

        if (project.getBasePath() != null) {
            command.withWorkDirectory(project.getBasePath());
        }

        handler = new OSProcessHandler(command);
        handler.startNotify();
        return true;
    }

    public synchronized boolean isRunning() {
        return handler != null && !handler.isProcessTerminated();
    }

    @Override
    public synchronized void dispose() {
        if (handler != null && !handler.isProcessTerminated()) {
            handler.destroyProcess();
        }
        handler = null;
    }

    public static @NotNull ControlPlaneProcessService getInstance(@NotNull Project project) {
        return project.getService(ControlPlaneProcessService.class);
    }
}
