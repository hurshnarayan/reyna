import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.ksp)
}

android {
    namespace = "app.reyna"
    compileSdk = 35

    val localProps = Properties()
    rootProject.file("local.properties").takeIf { it.exists() }?.inputStream()?.use {
        localProps.load(it)
    }

    defaultConfig {
        applicationId = "app.reyna"
        // 29 (Android 10) is the floor: WhatsApp moved its media into
        // Android/media/ around then, and that path is what capture depends on.
        minSdk = 29
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // Where the backend lives, and how to authenticate to it.
        //
        // Both come from android/local.properties, which is not in git, so a
        // build can be pointed at a real machine and carry its token without
        // either value ever being committed. Falling back to the emulator alias
        // and an empty token means a checkout with no local.properties still
        // builds and simply asks the user to fill Settings in.
        //
        // Baking the token in is a convenience for a personal build, not a
        // security design: anyone holding the APK holds the token, so a build
        // made this way should not be handed to people you would not give the
        // token to.
        buildConfigField(
            "String", "BACKEND_URL",
            "\"${localProps.getProperty("reyna.backendUrl") ?: "http://10.0.2.2:8080"}\"",
        )
        buildConfigField(
            "String", "DEVICE_TOKEN",
            "\"${localProps.getProperty("reyna.deviceToken") ?: ""}\"",
        )
    }

    // Signing for a release build, from local.properties so no key material is
    // committed. Absent that file the release build simply is not signed and
    // gradle says so, rather than silently producing something uninstallable.
    val keystorePath = localProps.getProperty("reyna.keystore")
    signingConfigs {
        if (keystorePath != null && file(keystorePath).exists()) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = localProps.getProperty("reyna.keystorePassword")
                keyAlias = localProps.getProperty("reyna.keyAlias")
                keyPassword = localProps.getProperty("reyna.keyPassword")
            }
        }
    }

    buildTypes {
        release {
            // A debug-signed APK asking for all-files and notification access
            // is exactly the shape Play Protect refuses to install when
            // sideloaded. Signing it with a stable key of our own does not make
            // Google trust it, but it stops the build from being the obvious
            // reason it was rejected, and it means updates install over each
            // other instead of colliding on a changed signature.
            signingConfig = signingConfigs.findByName("release")
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.ui)
    implementation(libs.androidx.ui.graphics)
    implementation(libs.androidx.ui.tooling.preview)
    implementation(libs.androidx.material3)
    implementation(libs.androidx.material.icons.extended)

    implementation(libs.androidx.room.runtime)
    implementation(libs.androidx.room.ktx)
    ksp(libs.androidx.room.compiler)

    implementation(libs.androidx.work.runtime.ktx)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.okhttp)
    implementation(libs.mlkit.text.recognition)
    implementation(libs.play.services.tasks)

    testImplementation(libs.junit)
}
