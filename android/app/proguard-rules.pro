# RaiseSteady Guard release rules (R8). Keep this short; every rule says why.

# pcap4j finds its packet factories and packet classes by reflection (StaticPacketFactoryBinder).
-keep class org.pcap4j.** { *; }
# pcap4j's native capture (JNA) and its optional logging back ends are not shipped.
-dontwarn com.sun.jna.**
-keep class com.sun.jna.Platform { *; }
-dontwarn org.pcap4j.core.**
-dontwarn ch.qos.logback.**

# dnsjava (as AdAway's rules): optional annotations and the JDK name-service SPI, not on Android.
-dontwarn lombok.Generated
-dontwarn sun.net.spi.nameservice.**
-dontwarn org.xbill.DNS.spi.**
-dontwarn javax.naming.**

# OkHttp: optional TLS providers that are not on Android.
-dontwarn org.bouncycastle.jsse.**
-dontwarn org.conscrypt.**
-dontwarn org.openjsse.**
