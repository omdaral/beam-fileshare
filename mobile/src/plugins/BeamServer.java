package app.beam.share;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.app.ActivityManager;
import android.net.Uri;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.PowerManager;
import android.provider.Settings;

import androidx.annotation.NonNull;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import beamapp.Beamapp;

import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Starts/stops the on-phone Beam server (Go engine via gomobile).
 *
 * Survival design (why the server used to die in the background):
 * - start/stop/health run on a background thread — Beamapp.start blocks up
 *   to ~5s and Shutdown up to ~3s, which ANR-killed the app on the main
 *   thread.
 * - While running we hold a high-perf WifiLock + partial WakeLock +
 *   MulticastLock natively (the JS screen wake-lock dies with the screen;
 *   only these keep CPU/WiFi alive for transfers with the screen off). The
 *   "Beam ..." notification itself is still shown by the JS foreground
 *   service call.
 * - The live LAN address is pushed to Go on start and on every network
 *   change (Android has no `ip`/`hostname` CLIs, so Go-side discovery alone
 *   reports stale addresses after WiFi <-> hotspot switches).
 * - Background survival is a chain, every link verified from JS:
 *   Beamapp.start persists "server_wanted" (wasWanted) so a process death
 *   auto-resumes on next launch; batteryExempt/requestBatteryExemption fight
 *   Doze + OEM killers; fgsAlive lets the UI verify the foreground-service
 *   notice really posted (a rejected/silent FGS used to fake success).
 */
@CapacitorPlugin(name = "BeamServer")
public class BeamServer extends Plugin {
    private static final int PORT = 2004;
    private static final String PREFS = "beam";
    private static final String KEY_WANTED = "server_wanted";
    private static final String FGS_CLASS =
            "io.capawesome.capacitorjs.plugins.foregroundservice.AndroidForegroundService";

    private final ExecutorService bg = Executors.newSingleThreadExecutor();
    private android.net.wifi.WifiManager.MulticastLock multicastLock;
    private WifiManager.WifiLock wifiLock;
    private PowerManager.WakeLock wakeLock;
    private ConnectivityManager.NetworkCallback networkCallback;

    private void locksOn() {
        try {
            Context app = getContext().getApplicationContext();
            WifiManager wm = (WifiManager) app.getSystemService(Context.WIFI_SERVICE);
            if (wm != null) {
                if (wifiLock == null) {
                    wifiLock = wm.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "beam:server");
                    wifiLock.setReferenceCounted(false);
                }
                if (!wifiLock.isHeld()) {
                    wifiLock.acquire();
                }
                if (multicastLock == null) {
                    multicastLock = wm.createMulticastLock("beam-mdns");
                    multicastLock.setReferenceCounted(true);
                }
                if (!multicastLock.isHeld()) {
                    multicastLock.acquire();
                }
            }
        } catch (Exception ignored) {
        }
        try {
            if (wakeLock == null) {
                PowerManager pm = (PowerManager) getContext().getApplicationContext()
                        .getSystemService(Context.POWER_SERVICE);
                if (pm != null) {
                    wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "beam:server");
                    wakeLock.setReferenceCounted(false);
                }
            }
            if (wakeLock != null && !wakeLock.isHeld()) {
                wakeLock.acquire();
            }
        } catch (Exception ignored) {
        }
    }

    private void locksOff() {
        try {
            if (multicastLock != null && multicastLock.isHeld()) {
                multicastLock.release();
            }
        } catch (Exception ignored) {
        }
        try {
            if (wifiLock != null && wifiLock.isHeld()) {
                wifiLock.release();
            }
        } catch (Exception ignored) {
        }
        try {
            if (wakeLock != null && wakeLock.isHeld()) {
                wakeLock.release();
            }
        } catch (Exception ignored) {
        }
    }

    private static int ipGroup(String ip) {
        if (ip.startsWith("10.42.0.") || ip.startsWith("192.168.137.")) return 0;
        if (ip.startsWith("192.168.") || ip.startsWith("10.") || ip.startsWith("172.")) return 1;
        return 2;
    }

    /** Best-effort live LAN address, hotspot-first like the Go side. */
    private String currentLanIp() {
        List<String> ips = new ArrayList<>();
        try {
            WifiManager wm = (WifiManager) getContext().getApplicationContext()
                    .getSystemService(Context.WIFI_SERVICE);
            if (wm != null && wm.getConnectionInfo() != null) {
                int raw = wm.getConnectionInfo().getIpAddress();
                if (raw != 0) {
                    ips.add(String.format(Locale.US, "%d.%d.%d.%d",
                            (raw & 0xff), (raw >> 8 & 0xff),
                            (raw >> 16 & 0xff), (raw >> 24 & 0xff)));
                }
            }
        } catch (Exception ignored) {
        }
        try {
            for (NetworkInterface ni : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                try {
                    if (!ni.isUp() || ni.isLoopback()) continue;
                } catch (Exception ignored) {
                    continue;
                }
                for (InetAddress a : Collections.list(ni.getInetAddresses())) {
                    if (a instanceof Inet4Address && !a.isLoopbackAddress()
                            && !a.isLinkLocalAddress()) {
                        String s = a.getHostAddress();
                        if (s != null && !s.isEmpty() && !ips.contains(s)) {
                            ips.add(s);
                        }
                    }
                }
            }
        } catch (Exception ignored) {
        }
        if (ips.isEmpty()) return "";
        Collections.sort(ips, (x, y) -> {
            int gx = ipGroup(x), gy = ipGroup(y);
            if (gx != gy) return gx - gy;
            return x.compareTo(y);
        });
        return ips.get(0);
    }

    private void pushLanIp() {
        try {
            String ip = currentLanIp();
            if (ip != null && !ip.isEmpty()) {
                Beamapp.setLanIp(ip);
            }
        } catch (Exception ignored) {
        }
    }

    /** Re-push the LAN address whenever the network changes (WiFi<->hotspot). */
    private void watchNetwork() {
        if (Build.VERSION.SDK_INT < 24) return;
        try {
            ConnectivityManager cm = (ConnectivityManager) getContext()
                    .getSystemService(Context.CONNECTIVITY_SERVICE);
            if (cm == null) return;
            unwatchNetwork();
            networkCallback = new ConnectivityManager.NetworkCallback() {
                @Override public void onAvailable(@NonNull Network network) { pushLanIp(); }
                @Override public void onLinkPropertiesChanged(@NonNull Network network,
                        LinkProperties lp) { pushLanIp(); }
                @Override public void onLost(@NonNull Network network) { pushLanIp(); }
            };
            cm.registerDefaultNetworkCallback(networkCallback);
        } catch (Exception ignored) {
        }
    }

    private void unwatchNetwork() {
        if (networkCallback == null) return;
        try {
            ConnectivityManager cm = (ConnectivityManager) getContext()
                    .getSystemService(Context.CONNECTIVITY_SERVICE);
            if (cm != null) {
                cm.unregisterNetworkCallback(networkCallback);
            }
        } catch (Exception ignored) {
        } finally {
            networkCallback = null;
        }
    }

    private String filesRoot() {
        return getContext().getFilesDir().getAbsolutePath();
    }

    /** Persisted "server should be running" flag — survives process death. */
    private void setWanted(boolean wanted) {
        try {
            SharedPreferences prefs = getContext().getSharedPreferences(PREFS, Context.MODE_PRIVATE);
            prefs.edit().putBoolean(KEY_WANTED, wanted).apply();
        } catch (Exception ignored) {
        }
    }

    /** Was the server wanted? Lets the UI auto-resume after a process kill. */
    @PluginMethod
    public void wasWanted(PluginCall call) {
        boolean wanted = false;
        try {
            wanted = getContext().getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                    .getBoolean(KEY_WANTED, false);
        } catch (Exception ignored) {
        }
        JSObject ret = new JSObject();
        ret.put("wanted", wanted);
        call.resolve(ret);
    }

    /** Is the app exempt from Doze/App-Standby battery restrictions? */
    @PluginMethod
    public void batteryExempt(PluginCall call) {
        boolean exempt = true; // pre-M has no Doze to fight
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                PowerManager pm = (PowerManager) getContext().getApplicationContext()
                        .getSystemService(Context.POWER_SERVICE);
                exempt = pm == null || pm.isIgnoringBatteryOptimizations(
                        getContext().getPackageName());
            }
        } catch (Exception ignored) {
        }
        JSObject ret = new JSObject();
        ret.put("exempt", exempt);
        call.resolve(ret);
    }

    /** Open the system dialog asking for battery-optimizations exemption. */
    @PluginMethod
    public void requestBatteryExemption(PluginCall call) {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                PowerManager pm = (PowerManager) getContext().getApplicationContext()
                        .getSystemService(Context.POWER_SERVICE);
                String pkg = getContext().getPackageName();
                if (pm == null || !pm.isIgnoringBatteryOptimizations(pkg)) {
                    Intent intent = new Intent(
                            Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                            Uri.parse("package:" + pkg));
                    intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                    getContext().startActivity(intent);
                }
            }
            call.resolve();
        } catch (Exception e) {
            call.reject(e.getMessage() != null ? e.getMessage() : "battery_request_failed");
        }
    }

    /** Is our foreground-service notice actually posted right now? */
    @PluginMethod
    public void fgsAlive(PluginCall call) {
        boolean alive = false;
        try {
            ActivityManager am = (ActivityManager) getContext()
                    .getSystemService(Context.ACTIVITY_SERVICE);
            if (am != null) {
                for (ActivityManager.RunningServiceInfo info
                        : am.getRunningServices(Integer.MAX_VALUE)) {
                    if (info != null && info.service != null
                            && FGS_CLASS.equals(info.service.getClassName())
                            && info.foreground) {
                        alive = true;
                        break;
                    }
                }
            }
        } catch (Exception ignored) {
        }
        JSObject ret = new JSObject();
        ret.put("alive", alive);
        call.resolve(ret);
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
        // Off the main thread: Beamapp.start blocks up to ~5s (ANR risk).
        bg.execute(() -> {
            String err;
            try {
                err = Beamapp.start(filesRoot(), (long) PORT, appVersion());
            } catch (Exception e) {
                call.reject(e.getMessage() != null ? e.getMessage() : "start_failed");
                return;
            }
            if (err != null && !err.isEmpty()) {
                call.reject(err);
                return;
            }
            setWanted(true); // process death must auto-resume on next launch
            locksOn(); // keep CPU/WiFi alive for transfers with screen off
            pushLanIp();
            watchNetwork();
            JSObject ret = new JSObject();
            ret.put("port", PORT);
            try {
                ret.put("shareDir", Beamapp.shareDir());
            } catch (Exception ignored) {
            }
            call.resolve(ret);
        });
    }

    @PluginMethod
    public void stop(PluginCall call) {
        // Off the main thread: Shutdown blocks up to ~3s (ANR risk).
        bg.execute(() -> {
            try {
                Beamapp.stop();
            } catch (Exception ignored) {
            }
            setWanted(false);
            unwatchNetwork();
            locksOff();
            call.resolve();
        });
    }

    @PluginMethod
    public void health(PluginCall call) {
        // Off the main thread: the self-probe does HTTP with a 4s budget.
        bg.execute(() -> {
            JSObject ret = new JSObject();
            try {
                ret.put("status", Beamapp.health((long) PORT));
            } catch (Exception e) {
                ret.put("status", "down");
            }
            try {
                ret.put("shareDir", Beamapp.shareDir());
            } catch (Exception ignored) {
            }
            call.resolve(ret);
        });
    }

    @PluginMethod
    public void shareDir(PluginCall call) {
        JSObject ret = new JSObject();
        ret.put("shareDir", Beamapp.shareDir());
        call.resolve(ret);
    }

    @PluginMethod
    public void ownerToken(PluginCall call) {
        JSObject ret = new JSObject();
        ret.put("token", Beamapp.ownerToken());
        call.resolve(ret);
    }

    /** Live LAN address as seen by Android (also re-pushed to Go). */
    @PluginMethod
    public void lanIp(PluginCall call) {
        bg.execute(() -> {
            String ip = currentLanIp();
            if (ip != null && !ip.isEmpty()) {
                try {
                    Beamapp.setLanIp(ip);
                } catch (Exception ignored) {
                }
            }
            JSObject ret = new JSObject();
            ret.put("ip", ip != null ? ip : "");
            call.resolve(ret);
        });
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

    @Override
    protected void handleOnDestroy() {
        unwatchNetwork();
        // NOTE: locks stay held if the server still runs — releasing them
        // here would silently drop background-transfer protection while the
        // Go listener keeps serving. stop() releases them explicitly.
    }
}
