package com.junefree.touchytails

import android.annotation.SuppressLint
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.BluetoothStatusCodes
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.SystemClock
import android.util.Log
import java.util.UUID

/**
 * One GATT connection with automatic reconnect.
 *
 * All state is touched only on [handler]'s thread. Android allows a single
 * outstanding GATT operation, so writes go one at a time: haptic values are
 * coalesced (only the latest is sent, write-without-response), and a "ping"
 * written with response every 2 s detects a dead link. On any failure the
 * BluetoothGatt is closed - not just disconnected - otherwise Android leaks
 * the client slot and reconnects stop working until the app is restarted.
 */
@SuppressLint("MissingPermission")
class BleLink(
    private val context: Context,
    val address: String,
    private val handler: Handler,
    private val onStatus: (address: String, status: String) -> Unit,
) {
    companion object {
        private const val TAG = "BleLink"
        val SERVICE: UUID = UUID.fromString("0000ab00-0000-1000-8000-00805f9b34fb")
        val CHAR: UUID = UUID.fromString("0000ab01-0000-1000-8000-00805f9b34fb")

        private const val CONNECT_TIMEOUT_MS = 15_000L
        private const val RECONNECT_DELAY_MS = 2_000L
        private const val PING_INTERVAL_MS = 2_000L
        private const val OP_TIMEOUT_MS = 3_000L
        private const val MIN_WRITE_GAP_MS = 20L
    }

    private var gatt: BluetoothGatt? = null
    private var char: BluetoothGattCharacteristic? = null
    private var ready = false
    private var closed = false

    private var pending: String? = null
    private var busy = false
    private var busySince = 0L
    private var lastWrite = 0L

    private val connectTimeout = Runnable { fail("connect timeout") }
    private val reconnect = Runnable { connect() }
    private val tick = object : Runnable {
        override fun run() {
            onTick()
            handler.postDelayed(this, PING_INTERVAL_MS)
        }
    }
    private val pumpLater = Runnable { pump() }

    fun start() = handler.post { closed = false; connect() }

    fun stop() = handler.post {
        closed = true
        teardown()
        onStatus(address, "Disabled")
    }

    /** Queue the latest value; older unsent values are dropped. */
    fun send(value: String) = handler.post {
        pending = value
        pump()
    }

    // ---------------- connection ----------------

    private fun connect() {
        if (closed || gatt != null) return
        onStatus(address, "Connecting")
        val adapter = context.getSystemService(BluetoothManager::class.java)?.adapter
        if (adapter == null || !adapter.isEnabled) {
            onStatus(address, "Bluetooth off")
            handler.postDelayed(reconnect, RECONNECT_DELAY_MS * 2)
            return
        }
        val device = adapter.getRemoteDevice(address)
        gatt = device.connectGatt(context, false, callback, BluetoothDevice.TRANSPORT_LE)
        handler.postDelayed(connectTimeout, CONNECT_TIMEOUT_MS)
    }

    private fun onReady(g: BluetoothGatt) {
        val c = g.getService(SERVICE)?.getCharacteristic(CHAR)
        if (c == null) {
            fail("characteristic not found")
            return
        }
        char = c
        ready = true
        handler.removeCallbacks(connectTimeout)
        handler.removeCallbacks(tick)
        handler.postDelayed(tick, PING_INTERVAL_MS)
        onStatus(address, "Online")
        pump()
    }

    private fun fail(reason: String) {
        Log.w(TAG, "$address: $reason")
        teardown()
        if (!closed) {
            onStatus(address, "Offline")
            handler.postDelayed(reconnect, RECONNECT_DELAY_MS)
        }
    }

    private fun teardown() {
        handler.removeCallbacks(connectTimeout)
        handler.removeCallbacks(reconnect)
        handler.removeCallbacks(tick)
        handler.removeCallbacks(pumpLater)
        ready = false
        busy = false
        char = null
        gatt?.let {
            try {
                it.disconnect()
            } finally {
                it.close()
            }
        }
        gatt = null
    }

    // ---------------- writes ----------------

    private fun pump() {
        if (!ready || busy) return
        val value = pending ?: return
        val wait = MIN_WRITE_GAP_MS - (SystemClock.uptimeMillis() - lastWrite)
        if (wait > 0) {
            handler.removeCallbacks(pumpLater)
            handler.postDelayed(pumpLater, wait)
            return
        }
        pending = null
        write(value, BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE)
    }

    private fun onTick() {
        if (!ready) return
        val now = SystemClock.uptimeMillis()
        if (busy) {
            if (now - busySince > OP_TIMEOUT_MS) fail("write timeout")
            return
        }
        if (now - lastWrite >= PING_INTERVAL_MS) {
            write("ping", BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT)
        }
    }

    @Suppress("DEPRECATION")
    private fun write(value: String, type: Int) {
        val g = gatt ?: return
        val c = char ?: return
        val bytes = value.toByteArray()
        val ok = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            g.writeCharacteristic(c, bytes, type) == BluetoothStatusCodes.SUCCESS
        } else {
            c.writeType = type
            c.value = bytes
            g.writeCharacteristic(c)
        }
        if (!ok) {
            fail("write rejected")
            return
        }
        busy = true
        busySince = SystemClock.uptimeMillis()
        lastWrite = busySince
    }

    // ---------------- GATT callbacks (binder thread -> handler) ----------------

    private val callback = object : BluetoothGattCallback() {
        override fun onConnectionStateChange(g: BluetoothGatt, status: Int, newState: Int) {
            handler.post {
                if (g !== gatt) {
                    g.close() // stale object from a previous attempt
                    return@post
                }
                if (status == BluetoothGatt.GATT_SUCCESS && newState == BluetoothProfile.STATE_CONNECTED) {
                    g.requestConnectionPriority(BluetoothGatt.CONNECTION_PRIORITY_HIGH)
                    if (!g.discoverServices()) fail("discoverServices failed")
                } else {
                    fail("state=$newState status=$status")
                }
            }
        }

        override fun onServicesDiscovered(g: BluetoothGatt, status: Int) {
            handler.post {
                if (g !== gatt) return@post
                if (status == BluetoothGatt.GATT_SUCCESS) onReady(g) else fail("discovery status=$status")
            }
        }

        override fun onCharacteristicWrite(g: BluetoothGatt, c: BluetoothGattCharacteristic, status: Int) {
            handler.post {
                if (g !== gatt) return@post
                busy = false
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail("write status=$status")
                    return@post
                }
                pump()
            }
        }
    }
}
