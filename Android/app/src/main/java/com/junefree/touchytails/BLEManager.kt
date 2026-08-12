package com.junefree.touchytails

import android.Manifest
import android.annotation.SuppressLint
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothManager
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanResult
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat

class BleManager(
    private val context: Context
) {
    companion object {
        const val TARGET_NAME = "TouchyTails"
    }

    private val bluetoothManager =
        context.getSystemService(BluetoothManager::class.java)

    private val adapter =
        bluetoothManager.adapter

    private val scanner: BluetoothLeScanner
        get() = adapter.bluetoothLeScanner

    private var scanning = false

    private val discoveredDevices = mutableMapOf<String, BluetoothDevice>()

    var onDeviceFound: ((BluetoothDevice) -> Unit)? = null
    var onScanFinished: (() -> Unit)? = null

    private val scanCallback = object : ScanCallback() {

        override fun onScanResult(
            callbackType: Int,
            result: ScanResult
        ) {
            val device = result.device

            // Only care about TouchyTails.
            if (device.name != TARGET_NAME) {
                return
            }

            // Use address as a temporary unique key.
            if (discoveredDevices.containsKey(device.address)) {
                return
            }

            discoveredDevices[device.address] = device

            onDeviceFound?.invoke(device)
        }

        override fun onScanFailed(errorCode: Int) {
            scanning = false
            onScanFinished?.invoke()
        }
    }

    fun hasPermissions(): Boolean {
        return ContextCompat.checkSelfPermission(
            context,
            Manifest.permission.BLUETOOTH_SCAN
        ) == PackageManager.PERMISSION_GRANTED &&
                ContextCompat.checkSelfPermission(
                    context,
                    Manifest.permission.BLUETOOTH_CONNECT
                ) == PackageManager.PERMISSION_GRANTED
    }

    @SuppressLint("MissingPermission")
    fun startScan() {
        if (!hasPermissions()) {
            return
        }

        if (scanning) {
            return
        }

        discoveredDevices.clear()
        scanning = true

        scanner.startScan(scanCallback)
    }

    @SuppressLint("MissingPermission")
    fun stopScan() {
        if (!hasPermissions()) {
            return
        }

        if (!scanning) {
            return
        }

        scanner.stopScan(scanCallback)
        scanning = false

        onScanFinished?.invoke()
    }
}