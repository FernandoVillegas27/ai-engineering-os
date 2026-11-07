package com.fernandovillegas.aiengineeringos;

import com.intellij.ide.BrowserUtil;
import com.intellij.openapi.actionSystem.AnAction;
import com.intellij.openapi.actionSystem.AnActionEvent;
import com.intellij.openapi.ui.Messages;
import org.jetbrains.annotations.NotNull;

public final class OpenControlPlaneAction extends AnAction {
    @Override
    public void actionPerformed(@NotNull AnActionEvent event) {
        try {
            AiEngineeringOsSettings settings = AiEngineeringOsSettings.getInstance();
            ControlPlaneAddress address = ControlPlaneAddress.parse(settings.controlPlaneAddress());
            BrowserUtil.browse(address.dashboardUri());
        } catch (IllegalArgumentException error) {
            Messages.showErrorDialog(
                    event.getProject(),
                    error.getMessage(),
                    "AI Engineering OS"
            );
        }
    }
}
