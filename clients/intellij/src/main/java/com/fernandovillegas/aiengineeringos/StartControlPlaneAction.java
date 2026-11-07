package com.fernandovillegas.aiengineeringos;

import com.intellij.execution.ExecutionException;
import com.intellij.openapi.actionSystem.AnAction;
import com.intellij.openapi.actionSystem.AnActionEvent;
import com.intellij.openapi.project.Project;
import com.intellij.openapi.ui.Messages;
import org.jetbrains.annotations.NotNull;

public final class StartControlPlaneAction extends AnAction {
    @Override
    public void update(@NotNull AnActionEvent event) {
        event.getPresentation().setEnabled(event.getProject() != null);
    }

    @Override
    public void actionPerformed(@NotNull AnActionEvent event) {
        Project project = event.getProject();
        if (project == null) return;

        try {
            ControlPlaneProcessService service = ControlPlaneProcessService.getInstance(project);
            boolean started = service.start();
            if (started) {
                Messages.showInfoMessage(
                        project,
                        "The local Control Plane was started. Use Tools → AI Engineering OS: Open Control Plane to inspect durable runs.",
                        "AI Engineering OS"
                );
            } else {
                Messages.showInfoMessage(
                        project,
                        "The Control Plane process started by this project is already running.",
                        "AI Engineering OS"
                );
            }
        } catch (ExecutionException | IllegalArgumentException error) {
            Messages.showErrorDialog(
                    project,
                    error.getMessage(),
                    "AI Engineering OS"
            );
        }
    }
}
