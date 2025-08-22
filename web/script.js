let gattBusy = false;
$(function(){
        // Request Bluetooth device
		if (typeof(navigator.bluetooth)!='undefined') {
			output("BLE ready","green");
		} else {
			output("BLE is not available<br>Consider using chrome browser or bluefy app","orange");
		}
		
		device = null;
		server = null;
		service = null;
		characteristic = null;
		
		$('#send').click(async function () {
			let data = $("#content").val();
			await safeWrite(characteristic, data);
			return false;
		});
		
		$('#connect').click(async function(){
			output("Connecting?", "green");
			try{

				try {
					device = await navigator.bluetooth.requestDevice({
						acceptAllDevices: true,
						optionalServices: [0xAB00],
					});
				} catch (error) {
					// If there is an error, try to connect without optional services
					device = await navigator.bluetooth.requestDevice({
						acceptAllDevices: true,
					});
				}

				// Connect to the device
				server = await device.gatt.connect();
				output('Device: '+server.device.name+' is connected');
				
				// Get the primary service
				service = await server.getPrimaryService(0xAB00);
				//output('Main service:<br>'+service.uuid);
				
				characteristic = await service.getCharacteristic(0xAB01);

				//read(characteristic);
				listen(characteristic);
			}catch(e){
				output(e, "green");
			}
			
			return false;
		});
		window.addEventListener("resize", function(){
			window.resizeTo(420, 500);
	});				
});
async function output(message, color=''){
	const now = new Date();
	const timeString = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`;
	$('.status').prepend('<div class="message '+color+'">'+timeString+' '+message+'</div>');
}


async function safeWrite(characteristic, value) {
	if (gattBusy) {
		output("GATT busy, write skipped", "orange");
		return;
	}
	gattBusy = true;

	try {
		var dataToWrite = new TextEncoder().encode(value);
		await characteristic.writeValue(dataToWrite);
		output("Written: " + value);
	} catch (err) {
		output("Write error: " + err, "red");
	} finally {
		gattBusy = false;
	}
}

async function safeRead(characteristic) {
	if (gattBusy) {
		output("GATT busy, read skipped", "orange");
		return;
	}
	gattBusy = true;

	try {
		let value = await characteristic.readValue();
		let decodedValue = new TextDecoder().decode(value);
		output("Read: " + decodedValue);
	} catch (err) {
		output("Read error: " + err, "red");
	} finally {
		gattBusy = false;
	}
}
async function listen(characteristic){
	characteristic.addEventListener('characteristicvaluechanged', async (event) => {
		if (gattBusy) return; // skip if a write is ongoing

		gattBusy = true;
		try {
			let value = event.target.value;
			let decodedValue = new TextDecoder().decode(value);
			output("Data: " + decodedValue, "green");
		} catch (err) {
			output("Notification error: " + err, "red");
		} finally {
			gattBusy = false;
		}
	});
	await characteristic.startNotifications();
	output("Notifications started", "green");
}