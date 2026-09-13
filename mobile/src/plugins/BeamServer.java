package app.beam.share;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import beamapp.Beamapp;

/** Starts/stops the on-phone Beam server (Go engine via gomobile). */
@CapacitorPlugin(name = "BeamServer")
public class BeamServer extends Plugin {
    private static final int PORT = 2004;
    private android.net.wifi.WifiManager.MulticastLock multicastLock;

    private void multicastOn() {
        try {
            if (multicastLock == null) {
                android.net.wifi.WifiManager wm = (android.net.wifi.WifiManager)
                        getContext().getApplicationContext()
                                .getSystemService(android.content.Context.WIFI_SERVICE);
                if (wm != null) {
                    multicastLock = wm.createMulticastLock("beam-mdns");
                    multicastLock.setReferenceCounted(true);
                }
            }
            if (multicastLock != null && !multicastLock.isHeld()) {
                multicastLock.acquire();
            }
        } catch (Exception ignored) {
        }
    }

    private void multicastOff() {
        try {
            if (multicastLock != null && multicastLock.isHeld()) {
                multicastLock.release();
            }
        } catch (Exception ignored) {
        }
    }

    private String filesRoot() {
        return getContext().getFilesDir().getAbsolutePath();
    }

    private String appVersion() {
        try {
            return getContext().getPackageManager()
                    .getPackageInfo(getContext().getPackageName(), 0).versionName;
        } catch (Exception e) {
            return "1.6.0";
        }
    }

    @PluginMethod
    public void start(PluginCall call) {
        String err = Beamapp.start(filesRoot(), (long) PORT, appVersion());
        if (err != null && !err.isEmpty()) {
            call.reject(err);
            return;
        }
        multicastOn(); // let mDNS/LLMNR queries reach the Go responders
        JSObject ret = new JSObject();
        ret.put("port", PORT);
        ret.put("shareDir", Beamapp.shareDir());
        call.resolve(ret);
    }

    @PluginMethod
    public void stop(PluginCall call) {
        Beamapp.stop();
        multicastOff();
        call.resolve();
    }

    @PluginMethod
    public void health(PluginCall call) {
        JSObject ret = new JSObject();
        ret.put("status", Beamapp.health((long) PORT));
        ret.put("shareDir", Beamapp.shareDir());
        call.resolve(ret);
    }

    @PluginMethod
    public void shareDir(PluginCall call) {
        JSObject ret = new JSObject();
        ret.put("shareDir", Beamapp.shareDir());
        call.resolve(ret);
    }

    @PluginMethod
    public void setHotspotCreds(PluginCall call) {
        String ssid = call.getString("ssid", "");
        String pass = call.getString("pass", "");
        String security = call.getString("security", "wpa");
        Beamapp.setHotspotCreds(ssid, pass, security);
        call.resolve();
    }

    @PluginMethod
    public void clearHotspot(PluginCall call) {
        Beamapp.clearHotspot();
        call.resolve();
    }
}
