package io.github.programgames.savesync

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.luminance
import org.junit.Assert.assertTrue
import org.junit.Test

// The State colours keep a text contrast of 4.5:1 (WCAG AA, small text) in both themes: on the surface (details), inside
// their tinted label (list) and, for In sync, on the selected row of the History ("Current").
class ColorsTest {
    private fun contrast(a: Color, b: Color): Double {
        val (hi, lo) = listOf(a.luminance(), b.luminance()).sortedDescending()
        return (hi + 0.05) / (lo + 0.05)
    }

    @Test
    fun stateColorsReadOnBothThemes() {
        for ((dark, scheme) in listOf(false to lightColorScheme(), true to darkColorScheme())) {
            for (s in Shown.entries) {
                val c = stateColor(s, dark) ?: scheme.onSurfaceVariant
                val label = c.copy(alpha = LABEL_TINT).compositeOver(scheme.surface)
                assertTrue("$s dark=$dark on surface", contrast(c, scheme.surface) >= 4.5)
                assertTrue("$s dark=$dark in its label: ${contrast(c, label)}", contrast(c, label) >= 4.5)
            }
            val sync = stateColor(Shown.IN_SYNC, dark)!!
            assertTrue("In sync dark=$dark on secondaryContainer", contrast(sync, scheme.secondaryContainer) >= 4.5)
        }
    }
}
