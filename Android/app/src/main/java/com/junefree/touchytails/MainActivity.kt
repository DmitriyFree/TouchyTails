package com.junefree.touchytails

import android.net.ConnectivityManager
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.junefree.touchytails.ui.theme.TouchyTailsTheme
import java.net.Inet4Address

class MainActivity : ComponentActivity() {

    private lateinit var scanner: BleManager
    private val found = mutableStateListOf<String>()
    private var scanning by mutableStateOf(false)
    private var message by mutableStateOf("")

    private val permissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
            if (Perms.granted(this)) {
                TailService.start(this)
            } else {
                message = "Bluetooth permissions are required"
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        DeviceRepo.init(this)

        scanner = BleManager(this).apply {
            onDeviceFound = { addr ->
                if (DeviceRepo.devices.value.none { it.address == addr } && addr !in found) found.add(addr)
            }
            onScanFinished = { scanning = false }
        }

        if (Perms.granted(this)) TailService.start(this)
        else permissionLauncher.launch(Perms.required())

        setContent {
            TouchyTailsTheme {
                Surface(Modifier.fillMaxSize()) { Screen() }
            }
        }
    }

    override fun onDestroy() {
        scanner.stopScan()
        super.onDestroy() // the service keeps running on purpose
    }

    private fun scan() {
        if (!Perms.granted(this)) {
            permissionLauncher.launch(Perms.required())
            return
        }
        if (!scanner.isBluetoothOn()) {
            message = "Turn Bluetooth on"
            return
        }
        found.clear()
        message = ""
        scanning = scanner.startScan()
        if (!scanning) message = "Scan failed to start (on Android 10/11 location must be on)"
    }

    private fun localIp(): String {
        val cm = getSystemService(ConnectivityManager::class.java)
        val lp = cm?.getLinkProperties(cm.activeNetwork) ?: return "no network"
        return lp.linkAddresses.map { it.address }.firstOrNull { it is Inet4Address }?.hostAddress
            ?: "no IPv4"
    }

    @Composable
    private fun Screen() {
        val devices by DeviceRepo.devices.collectAsState()
        val status by Hub.status.collectAsState()
        val lastOsc by Hub.lastOsc.collectAsState()
        val running by Hub.running.collectAsState()

        Column(
            Modifier.fillMaxSize().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text("TouchyTails", style = MaterialTheme.typography.headlineMedium)
            Text(
                "OSC: UDP ${TailService.OSC_PORT} · IP ${localIp()} · " +
                        if (running) "service running" else "service stopped",
                style = MaterialTheme.typography.bodySmall
            )
            if (lastOsc.isNotEmpty()) Text(lastOsc, style = MaterialTheme.typography.bodySmall)
            if (message.isNotEmpty()) Text(message, color = MaterialTheme.colorScheme.error)

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { if (scanning) scanner.stopScan() else scan() }) {
                    Text(if (scanning) "Stop scan" else "Scan")
                }
                OutlinedButton(onClick = {
                    if (running) TailService.stop(this@MainActivity)
                    else if (Perms.granted(this@MainActivity)) TailService.start(this@MainActivity)
                    else permissionLauncher.launch(Perms.required())
                }) { Text(if (running) "Stop service" else "Start service") }
            }

            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(found, key = { "new-$it" }) { addr ->
                    Card(Modifier.fillMaxWidth()) {
                        Row(
                            Modifier.padding(12.dp).fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("New: $addr", Modifier.weight(1f))
                            Button(onClick = { DeviceRepo.add(addr); found.remove(addr) }) { Text("Add") }
                        }
                    }
                }
                items(devices, key = { it.address }) { d ->
                    DeviceCard(d, status[d.address] ?: "…")
                }
            }
        }
    }

    @Composable
    private fun DeviceCard(d: DeviceConfig, status: String) {
        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(d.address, style = MaterialTheme.typography.bodySmall)
                        Text(status, color = statusColor(status))
                    }
                    Switch(
                        checked = d.enabled,
                        onCheckedChange = { on -> DeviceRepo.update(d.address) { it.copy(enabled = on) } }
                    )
                }
                OutlinedTextField(
                    value = d.name,
                    onValueChange = { v -> DeviceRepo.update(d.address) { it.copy(name = v) } },
                    label = { Text("Name") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                OutlinedTextField(
                    value = d.param,
                    onValueChange = { v -> DeviceRepo.update(d.address) { it.copy(param = v.trim()) } },
                    label = { Text("Avatar parameter (VRCContactReceiver)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Row {
                    TextButton(
                        onClick = { Hub.service?.test(d.address) },
                        enabled = status == "Online"
                    ) { Text("Test") }
                    Spacer(Modifier.weight(1f))
                    TextButton(onClick = { DeviceRepo.remove(d.address) }) { Text("Remove") }
                }
            }
        }
    }

    private fun statusColor(s: String) = when (s) {
        "Online" -> Color(0xFF4CAF50)
        "Connecting" -> Color(0xFFFFC107)
        "Disabled" -> Color.Gray
        else -> Color(0xFFE57373)
    }
}
