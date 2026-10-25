package com.junefree.touchytails

import android.Manifest
import android.annotation.SuppressLint
import android.bluetooth.BluetoothManager
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelUuid
import androidx.core.content.ContextCompat

/** Runtime permissions needed for BLE on this Android version. */
object Perms {
    fun required(): Array<String> = buildList {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            add(Manifest.permission.BLUETOOTH_SCAN)
            add(Manifest.permission.BLUETOOTH_CONNECT)
        } else {
            // Android 10/11 (Pico 4, older Quest firmware): scanning needs location
            add(Manifest.permission.ACCESS_FINE_LOCATION)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            add(Manifest.permission.POST_NOTIFICATIONS)
        }
    }.toTypedArray()

    fun granted(context: Context): Boolean = required()
        .filter { it != Manifest.permission.POST_NOTIFICATIONS } // optional
        .all { ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED }
}

/** Scans for TouchyTails devices (by service UUID or name). */
@SuppressLint("MissingPermission")
class BleManager(private val context: Context) {

    companion object {
        const val TARGET_NAME = "TouchyTails"
        private const val SCAN_MS = 10_000L
    }

    private val adapter = context.getSystemService(BluetoothManager::class.java)?.adapter
    private val main = Handler(Looper.getMainLooper())
    private val seen = mutableSetOf<String>()
    private var scanning = false

    var onDeviceFound: ((address: String) -> Unit)? = null
    var onScanFinished: (() -> Unit)? = null

    private val autoStop = Runnable { stopScan() }

    private val scanCallback = object : ScanCallback() {
        override fun onScanResult(callbackType: Int, result: ScanResult) {
            val name = result.scanRecord?.deviceName
            val hasService = result.scanRecord?.serviceUuids
                ?.contains(ParcelUuid(BleLink.SERVICE)) == true
            if (name != TARGET_NAME && !hasService) return
            val addr = result.device.address
            main.post {
                if (seen.add(addr)) onDeviceFound?.invoke(addr)
            }
        }

        override fun onScanFailed(errorCode: Int) {
            main.post { finish() }
        }
    }

    fun hasPermissions() = Perms.granted(context)

    fun isBluetoothOn() = adapter?.isEnabled == true

    fun startScan(): Boolean {
        if (!hasPermissions() || scanning) return false
        val scanner = adapter?.takeIf { it.isEnabled }?.bluetoothLeScanner ?: return false
        seen.clear()
        scanning = true
        val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
        // No filter: some firmwares only put the name in the scan response
        scanner.startScan(emptyList<ScanFilter>(), settings, scanCallback)
        main.postDelayed(autoStop, SCAN_MS)
        return true
    }

    fun stopScan() {
        if (!scanning) return
        try {
            adapter?.bluetoothLeScanner?.stopScan(scanCallback)
        } catch (_: Exception) {
        }
        finish()
    }

    private fun finish() {
        main.removeCallbacks(autoStop)
        scanning = false
        onScanFinished?.invoke()
    }
}
