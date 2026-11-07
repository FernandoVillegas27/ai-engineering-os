package com.fernandovillegas.aiengineeringos;

import com.intellij.openapi.options.Configurable;
import com.intellij.openapi.options.ConfigurationException;
import org.jetbrains.annotations.Nls;
import org.jetbrains.annotations.Nullable;

import javax.swing.*;
import java.awt.*;

public final class AiEngineeringOsConfigurable implements Configurable {
    private JPanel panel;
    private JTextField binaryField;
    private JTextField addressField;

    @Override
    public @Nls(capitalization = Nls.Capitalization.Title) String getDisplayName() {
        return "AI Engineering OS";
    }

    @Override
    public @Nullable JComponent createComponent() {
        panel = new JPanel(new GridBagLayout());
        binaryField = new JTextField();
        addressField = new JTextField();

        GridBagConstraints label = new GridBagConstraints();
        label.gridx = 0;
        label.anchor = GridBagConstraints.LINE_START;
        label.insets = new Insets(4, 4, 4, 12);

        GridBagConstraints field = new GridBagConstraints();
        field.gridx = 1;
        field.weightx = 1.0;
        field.fill = GridBagConstraints.HORIZONTAL;
        field.insets = new Insets(4, 4, 4, 4);

        label.gridy = 0;
        field.gridy = 0;
        panel.add(new JLabel("AI binary:"), label);
        panel.add(binaryField, field);

        label.gridy = 1;
        field.gridy = 1;
        panel.add(new JLabel("Control Plane address:"), label);
        panel.add(addressField, field);

        GridBagConstraints spacer = new GridBagConstraints();
        spacer.gridx = 0;
        spacer.gridy = 2;
        spacer.gridwidth = 2;
        spacer.weighty = 1.0;
        spacer.fill = GridBagConstraints.VERTICAL;
        panel.add(Box.createVerticalGlue(), spacer);

        reset();
        return panel;
    }

    @Override
    public boolean isModified() {
        if (binaryField == null || addressField == null) return false;
        AiEngineeringOsSettings settings = AiEngineeringOsSettings.getInstance();
        return !binaryField.getText().trim().equals(settings.binaryPath())
                || !addressField.getText().trim().equals(settings.controlPlaneAddress());
    }

    @Override
    public void apply() throws ConfigurationException {
        if (binaryField == null || addressField == null) return;
        try {
            ControlPlaneAddress.parse(addressField.getText());
        } catch (IllegalArgumentException error) {
            throw new ConfigurationException(error.getMessage());
        }
        AiEngineeringOsSettings.getInstance().update(binaryField.getText(), addressField.getText());
        reset();
    }

    @Override
    public void reset() {
        if (binaryField == null || addressField == null) return;
        AiEngineeringOsSettings settings = AiEngineeringOsSettings.getInstance();
        binaryField.setText(settings.binaryPath());
        addressField.setText(settings.controlPlaneAddress());
    }

    @Override
    public void disposeUIResources() {
        panel = null;
        binaryField = null;
        addressField = null;
    }
}
