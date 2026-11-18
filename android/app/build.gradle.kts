// Приложение «PC Block» для родителя (M7, T8). Протокол — docs/07_Протокол_связи.md.
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Версия программы — одна на компьютер и телефон: берём её из internal/version/version.go.
val versionFile = rootProject.file("../internal/version/version.go")
val appVersion: String = Regex("""Version\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+[^"]*)"""")
    .find(versionFile.readText())?.groupValues?.get(1)
    ?: throw GradleException("Не найдена версия в ${versionFile.path}")
val appVersionCode: Int = Regex("""^([0-9]+)\.([0-9]+)\.([0-9]+)""").find(appVersion)!!.destructured
    .let { (major, minor, patch) -> major.toInt() * 10000 + minor.toInt() * 100 + patch.toInt() }

// Подпись выпуска — только из секретов CI (D-94). Без них выпуск собирается неподписанным.
val keystoreFile: String? = System.getenv("ANDROID_KEYSTORE_FILE")?.takeIf { it.isNotBlank() }
val keystorePassword: String? = System.getenv("ANDROID_KEYSTORE_PASSWORD")?.takeIf { it.isNotBlank() }
val canSignRelease = keystoreFile != null && keystorePassword != null && file(keystoreFile).exists()

android {
    namespace = "ru.atlas404.pcblock"
    compileSdk = 35

    defaultConfig {
        applicationId = "ru.atlas404.pcblock"
        minSdk = 26
        targetSdk = 35
        versionCode = appVersionCode
        versionName = appVersion
    }

    signingConfigs {
        if (canSignRelease) {
            create("release") {
                storeFile = file(keystoreFile!!)
                storePassword = keystorePassword
                storeType = "PKCS12"
                keyAlias = "pcblock"
                keyPassword = keystorePassword
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (canSignRelease) {
                signingConfig = signingConfigs.getByName("release")
            }
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
    }

    // Векторы протокола из Go (testdata/remote/vectors.json) — ресурс JVM-тестов.
    sourceSets["test"].resources.srcDir("../../testdata/remote")

    lint {
        // Проверки lint не должны ронять сборку выпуска в CI: правим по отчёту, а не по падению.
        checkReleaseBuilds = false
        abortOnError = false
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-ktx:2.8.7")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core")

    // В Android org.json встроен в систему; в JVM-тестах нужна настоящая библиотека.
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
